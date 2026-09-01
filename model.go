package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	tea "charm.land/bubbletea/v2"
)

type mode int

const (
	modeNormal mode = iota
	modeComment
	modeHelp
	modeSearch
	modeConfirmDelete
	modeConfirmClearSession
)

// rowKind distinguishes the two kinds of row the diff pane can show.
type rowKind int

const (
	rowHunkHeader rowKind = iota
	rowLine
)

// diffRow is one flattened, cursor-addressable row of a file's diff: either
// a hunk header (not commentable) or a Line (commentable).
type diffRow struct {
	kind       rowKind
	hunkHeader string
	line       Line
}

func (r diffRow) isHeader() bool { return r.kind == rowHunkHeader }

// fileRows is a file's diff flattened into rows, precomputed once per file
// so cursor movement is just index arithmetic.
type fileRows struct {
	file FileDiff
	rows []diffRow
	// splitRows is rows' split-view counterpart (see splitpair.go) —
	// precomputed alongside rows so switching the split-view toggle is
	// just picking which of the two to use, not a re-parse.
	splitRows []pairedRow
	// numWidth is how many digits wide this file's line-number gutter
	// columns need to be — sized to the file's own largest line number
	// rather than a fixed width, so a 40-line file gets a 2-char gutter
	// instead of always reserving room for 4.
	numWidth int
	// lexer is resolved once per file (lexers.Match does filename pattern
	// matching — not worth repeating for every line rendered).
	lexer chroma.Lexer
}

func flattenFile(fd FileDiff) fileRows {
	fr := fileRows{file: fd, lexer: pickLexer(fd.Path)}
	maxNum := 0
	for _, h := range fd.Hunks {
		fr.rows = append(fr.rows, diffRow{kind: rowHunkHeader, hunkHeader: h.Header})
		for _, l := range h.Lines {
			fr.rows = append(fr.rows, diffRow{kind: rowLine, line: l})
			if l.OldLine != nil && *l.OldLine > maxNum {
				maxNum = *l.OldLine
			}
			if l.NewLine != nil && *l.NewLine > maxNum {
				maxNum = *l.NewLine
			}
		}
	}
	fr.numWidth = len(strconv.Itoa(maxNum))
	if fr.numWidth < 1 {
		fr.numWidth = 1
	}
	fr.splitRows = flattenFileSplit(fd)
	return fr
}

type model struct {
	repoRoot string
	diffSpec []string // what we're diffing, e.g. ["HEAD"] or ["main..feature"] — see gitVCS.DiffSpec
	files    []fileRows
	session  Session

	width, height int

	fileIndex  int
	lineIndex  int    // index into files[fileIndex].rows
	fileFilter string // confirmed sidebar filter (substring, case-insensitive on path); "" means show everything

	pendingG    bool   // mid-"gg" chord
	countBuffer string // digits typed so far for a pending vim-style count, e.g. "10" of "10j"

	mode  mode
	input string

	// editingCommentID/editingReplyID/replyingToCommentID are set instead of
	// "" while modeComment is editing an existing comment's or reply's body
	// in place, or adding a new reply to a comment, rather than composing a
	// brand-new top-level comment — see commentActionForCurrentLine and
	// addCommentUnderCursor. At most one of the three is ever set at a time.
	editingCommentID    string
	editingReplyID      string
	replyingToCommentID string

	// commentNavIncludeResolved widens n/N (jumpToComment) to also stop on
	// resolved comments instead of skipping them — toggled by
	// keys.ToggleCommentScope (rather than new movement keys, so n/N keep
	// their existing muscle memory), and persisted like the other display
	// prefs below.
	commentNavIncludeResolved bool

	// helpScroll is how many lines of helpRows() are scrolled past, while
	// mode == modeHelp — reset to 0 each time the overlay opens (see
	// keys.Help in updateNormal) so it never reopens mid-scroll from
	// wherever it was left last time.
	helpScroll int

	status string
	err    error

	keys Keymap

	// UI display prefs — persisted across sessions, see prefs.go.
	sidebarHidden   bool
	showLineNumbers bool
	wrapLines       bool
	showUntracked   bool
	// splitView shows old/new side by side (see splitpair.go) instead of
	// the unified single-column diff. Shares wrapLines with unified view —
	// each side of a paired row wraps independently within its own column
	// (see render.go's splitSidePhysicalLines).
	splitView bool

	// trackedDiffs is the `git diff <spec>` result — always shown.
	// untrackedDiffs is synthesized from files git isn't tracking at all
	// (see untrackedFileDiffs in diff.go) and only included in m.files when
	// showUntracked is on. Kept separately (rather than merging once into
	// m.files) so toggling showUntracked can rebuild m.files instantly from
	// what's already in memory, without a fresh git call — see
	// effectiveDiffFiles and (*model).setShowUntracked.
	trackedDiffs   []FileDiff
	untrackedDiffs []FileDiff

	// sessionMTime is the on-disk session's last-seen mtime, used to detect
	// changes made by another process (e.g. an agent running `rv comment
	// reply`) while this TUI instance is sitting open — see pollSessionCmd.
	sessionMTime time.Time
}

func newModel(repoRoot string, diffFiles []FileDiff, session Session, diffSpec []string) model {
	prefs := loadUIPrefs()
	m := model{
		repoRoot:                  repoRoot,
		diffSpec:                  diffSpec,
		trackedDiffs:              diffFiles,
		session:                   session,
		keys:                      defaultKeymap(),
		sidebarHidden:             prefs.SidebarHidden,
		showLineNumbers:           prefs.ShowLineNumbers,
		wrapLines:                 prefs.WrapLines,
		showUntracked:             prefs.ShowUntracked,
		commentNavIncludeResolved: prefs.CommentNavIncludeResolved,
		splitView:                 prefs.SplitView,
	}
	if mt, err := sessionModTime(repoRoot); err == nil {
		m.sessionMTime = mt
	}
	m.setDiffFiles(m.effectiveDiffFiles())
	return m
}

// effectiveDiffFiles is trackedDiffs, plus untrackedDiffs appended when
// showUntracked is on — the combined list setDiffFiles actually flattens
// into m.files.
func (m model) effectiveDiffFiles() []FileDiff {
	if !m.showUntracked || len(m.untrackedDiffs) == 0 {
		return m.trackedDiffs
	}
	out := make([]FileDiff, 0, len(m.trackedDiffs)+len(m.untrackedDiffs))
	out = append(out, m.trackedDiffs...)
	out = append(out, m.untrackedDiffs...)
	return out
}

// setUntrackedDiffs records freshly-fetched untracked-file diffs (initial
// load or refresh — see runTUI/(*model).refreshAll) and rebuilds m.files if
// they're currently shown.
func (m *model) setUntrackedDiffs(untracked []FileDiff) {
	m.untrackedDiffs = untracked
	m.setDiffFiles(m.effectiveDiffFiles())
}

// toggleShowUntracked flips whether untrackedDiffs are included in m.files
// — no fresh git call needed, just a rebuild from what's already fetched.
func (m *model) toggleShowUntracked() {
	m.showUntracked = !m.showUntracked
	m.setDiffFiles(m.effectiveDiffFiles())
}

// uiPrefs snapshots the model's persisted display prefs, for saving back
// to disk after any of them change.
func (m model) uiPrefs() uiPrefs {
	return uiPrefs{
		SidebarHidden:             m.sidebarHidden,
		ShowLineNumbers:           m.showLineNumbers,
		WrapLines:                 m.wrapLines,
		ShowUntracked:             m.showUntracked,
		CommentNavIncludeResolved: m.commentNavIncludeResolved,
		SplitView:                 m.splitView,
	}
}

func (m model) Init() tea.Cmd { return pollSessionCmd() }

// diffLabel is what's shown in the header for "what am I diffing" — e.g.
// "HEAD" (the default) or "main..feature".
func (m model) diffLabel() string {
	if len(m.diffSpec) == 0 {
		return "HEAD"
	}
	return strings.Join(m.diffSpec, " ")
}

// diffStats totals added/removed lines across every file in the diff —
// shown in the header as a whole-diff summary (e.g. "+123 -45"), the same
// stat GitHub/most git tools show for a whole PR/commit.
func (m model) diffStats() (added, removed int) {
	for _, fr := range m.files {
		a, r := fileDiffStats(fr.file)
		added += a
		removed += r
	}
	return added, removed
}

// fileDiffStats is diffStats' per-file counterpart, shown next to each
// sidebar row and for whichever file is currently selected.
func fileDiffStats(fd FileDiff) (added, removed int) {
	for _, h := range fd.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case LineAdded:
				added++
			case LineRemoved:
				removed++
			}
		}
	}
	return added, removed
}

// The cursor must never rest on a header row — you can't comment on a
// header, so stopping there is always a dead end that costs an extra
// keypress. Every path that sets lineIndex funnels through one of these
// three so that invariant holds everywhere, not just after }/{.
//
// Generified over contentRow so both the unified view's []diffRow and the
// split view's []pairedRow (see splitpair.go) share this logic instead of a
// forked copy.

// contentRow is the common shape firstContentRow/lastContentRow/
// nearestContentRow need: anything that can say whether it's a header (a
// dead end for the cursor) or actual content.
type contentRow interface {
	isHeader() bool
}

// firstContentRow returns the index of the first commentable row in rows,
// or 0 if there isn't one (shouldn't happen — every hunk has at least one
// line — but keeps callers safe regardless).
func firstContentRow[R contentRow](rows []R) int {
	for i, r := range rows {
		if !r.isHeader() {
			return i
		}
	}
	return 0
}

// lastContentRow is firstContentRow's counterpart.
func lastContentRow[R contentRow](rows []R) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].isHeader() {
			return i
		}
	}
	if len(rows) == 0 {
		return 0
	}
	return len(rows) - 1
}

// nearestContentRow returns idx (clamped into range) if it's already a
// commentable row, otherwise the closest one — searching forward first,
// since a "land roughly here" jump (e.g. {count}G on a header row) reading
// as "the line right after" is more intuitive than snapping backward.
func nearestContentRow[R contentRow](rows []R, idx int) int {
	if len(rows) == 0 {
		return 0
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if !rows[idx].isHeader() {
		return idx
	}
	for i := idx + 1; i < len(rows); i++ {
		if !rows[i].isHeader() {
			return i
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if !rows[i].isHeader() {
			return i
		}
	}
	return idx
}

// currentRows returns the rows for the currently selected file, or nil if
// there are no changed files.
func (m model) currentRows() []diffRow {
	if m.fileIndex < 0 || m.fileIndex >= len(m.files) {
		return nil
	}
	return m.files[m.fileIndex].rows
}

// currentSplitRows is currentRows' split-view counterpart.
func (m model) currentSplitRows() []pairedRow {
	if m.fileIndex < 0 || m.fileIndex >= len(m.files) {
		return nil
	}
	return m.files[m.fileIndex].splitRows
}

// setDiffFiles replaces the model's diff with a freshly re-parsed one (see
// refreshAll in update.go), keeping the cursor on the same file by path
// when it still exists, and clamping lineIndex if that file got shorter.
func (m *model) setDiffFiles(diffFiles []FileDiff) {
	currentPath := ""
	if m.fileIndex >= 0 && m.fileIndex < len(m.files) {
		currentPath = m.files[m.fileIndex].file.Path
	}

	files := make([]fileRows, 0, len(diffFiles))
	newIndex := 0
	for i, fd := range diffFiles {
		files = append(files, flattenFile(fd))
		if fd.Path == currentPath {
			newIndex = i
		}
	}
	m.files = files
	m.fileIndex = newIndex

	if len(files) == 0 {
		m.lineIndex = 0
		return
	}
	if m.splitView {
		m.lineIndex = nearestContentRow(files[newIndex].splitRows, m.lineIndex)
	} else {
		m.lineIndex = nearestContentRow(files[newIndex].rows, m.lineIndex)
	}
	m.snapToVisibleFile()
}

// visibleFileIndices returns indices into m.files matching query
// (case-insensitive substring on path) — or every index if query is empty.
// Used both to render a filtered sidebar and to know which files ]/tab/[
// may land the cursor on.
func (m model) visibleFileIndices(query string) []int {
	if query == "" {
		idx := make([]int, len(m.files))
		for i := range m.files {
			idx[i] = i
		}
		return idx
	}
	q := strings.ToLower(query)
	var idx []int
	for i, fr := range m.files {
		if strings.Contains(strings.ToLower(fr.file.Path), q) {
			idx = append(idx, i)
		}
	}
	return idx
}

// halfPageSize is how many rows ctrl+d/ctrl+u move, mirroring vim: half of
// the diff pane's visible height (the same height math render.go uses to
// size that panel).
func (m model) halfPageSize() int {
	h := m.bodyHeight() - borderOverheadH
	if h < 2 {
		return 1
	}
	return h / 2
}

// currentLine returns the Line under the cursor, if the cursor is on a
// commentable row.
func (m model) currentLine() (Line, bool) {
	if m.splitView {
		rows := m.currentSplitRows()
		if m.lineIndex < 0 || m.lineIndex >= len(rows) {
			return Line{}, false
		}
		row := rows[m.lineIndex]
		if row.kind != rowLine {
			return Line{}, false
		}
		// A genuine modified pair has both sides — prefer the new (right)
		// side, matching the "prefer new-side" convention already used
		// below in currentLineLabel and in lineNumberMatches; an
		// unmatched removed-only row has no right side, so fall back to
		// left.
		if row.right != nil {
			return *row.right, true
		}
		if row.left != nil {
			return *row.left, true
		}
		return Line{}, false
	}
	rows := m.currentRows()
	if m.lineIndex < 0 || m.lineIndex >= len(rows) {
		return Line{}, false
	}
	row := rows[m.lineIndex]
	if row.kind != rowLine {
		return Line{}, false
	}
	return row.line, true
}

// currentLineLabel describes where the cursor is within the current file —
// "42" for a line (preferring the new-side number, since that's the file
// as it exists now; falling back to old-side for a removed line, which has
// no new-side position), or "hunk" when it's sitting on a hunk header.
// Shown in the header so jumping around (}/{ especially) always tells you
// where you landed, independent of whether the line-number gutter (#) is
// even on.
func (m model) currentLineLabel() string {
	if m.splitView {
		rows := m.currentSplitRows()
		if m.lineIndex < 0 || m.lineIndex >= len(rows) {
			return ""
		}
		row := rows[m.lineIndex]
		if row.kind == rowHunkHeader {
			return "hunk"
		}
		if row.right != nil && row.right.NewLine != nil {
			return strconv.Itoa(*row.right.NewLine)
		}
		if row.left != nil && row.left.OldLine != nil {
			return strconv.Itoa(*row.left.OldLine)
		}
		return ""
	}
	rows := m.currentRows()
	if m.lineIndex < 0 || m.lineIndex >= len(rows) {
		return ""
	}
	row := rows[m.lineIndex]
	if row.kind == rowHunkHeader {
		return "hunk"
	}
	if row.line.NewLine != nil {
		return strconv.Itoa(*row.line.NewLine)
	}
	if row.line.OldLine != nil {
		return strconv.Itoa(*row.line.OldLine)
	}
	return ""
}

// isFileReviewed reports whether fd is marked reviewed in session AND its
// content still matches the hash it had when marked — a stale mark (the
// file changed since) counts as unreviewed rather than lying about it.
func isFileReviewed(session Session, fd FileDiff) bool {
	if session.Reviewed == nil {
		return false
	}
	hash, ok := session.Reviewed[fd.Path]
	return ok && hash == fileDiffHash(fd)
}

// commentsForCurrentLine returns comments anchored to the cursor's current
// row (see commentAnchorRow).
func (m model) commentsForCurrentLine() []Comment {
	if m.fileIndex < 0 || m.fileIndex >= len(m.files) {
		return nil
	}
	return m.commentsByRow(m.files[m.fileIndex])[m.lineIndex]
}

// locateComment finds where c's anchor lives in the currently loaded diff —
// which file, and which row within that file's flattened rows (see
// commentAnchorRow) — so the cursor can be moved directly to it (see
// (*model).jumpToComment).
func (m model) locateComment(c Comment) (fileIdx, rowIdx int, ok bool) {
	for fi, fr := range m.files {
		if m.splitView {
			if idx, anchored := commentAnchorRowSplit(fr, c); anchored {
				return fi, idx, true
			}
			continue
		}
		if idx, anchored := commentAnchorRow(fr, c); anchored {
			return fi, idx, true
		}
	}
	return 0, 0, false
}

// commentsByRow anchors every session comment belonging to fr's file to a
// row index (see commentAnchorRow), grouped by row — the per-file
// precomputation buildDiffLinesDetailed and commentsForCurrentLine both use,
// rather than re-resolving every comment's anchor on every row.
func (m model) commentsByRow(fr fileRows) map[int][]Comment {
	byRow := make(map[int][]Comment)
	for _, c := range m.session.Comments {
		var (
			idx int
			ok  bool
		)
		if m.splitView {
			idx, ok = commentAnchorRowSplit(fr, c)
		} else {
			idx, ok = commentAnchorRow(fr, c)
		}
		if ok {
			byRow[idx] = append(byRow[idx], c)
		}
	}
	return byRow
}

// lineNumberMatches is commentAnchorRow's line-number half: a comment
// matches on new-line number if it has one, otherwise on old-line number
// (mirroring how a comment is created in (*model).addCommentUnderCursor —
// a removed line has no new-side position, so that's the only case where
// the old-line fallback applies).
func lineNumberMatches(line Line, c Comment) bool {
	if line.NewLine != nil && c.NewLine != nil {
		return *line.NewLine == *c.NewLine
	}
	if line.OldLine != nil && c.OldLine != nil && line.NewLine == nil && c.NewLine == nil {
		return *line.OldLine == *c.OldLine
	}
	return false
}

// commentAnchorRow finds which row in fr's rows c should attach to now,
// tolerating the line having moved since the comment was created (an edit
// reverted, or unrelated lines shifted above it) instead of either silently
// losing the comment or — worse — silently reattaching it to a different,
// unrelated line that happens to land at the same number:
//
//  1. Its original file + line number, IF the row still there also still
//     has the exact content the comment recorded at creation time
//     (c.LineContent) — the common case, nothing moved.
//  2. Failing that, wherever in the file a row's content exactly matches
//     c.LineContent, but ONLY if that's true of exactly one row — an
//     ambiguous (or absent) match means guessing wrong is worse than not
//     guessing, so the comment is left anchored nowhere (orphaned; see
//     (m model).orphanedCommentCount) rather than reattached speculatively.
//
// c.LineContent == "" (a comment created before this existed) falls back
// to pure line-number matching — unchanged from the original behavior,
// since there's nothing to content-check against.
func commentAnchorRow(fr fileRows, c Comment) (rowIdx int, ok bool) {
	if fr.file.Path != c.File {
		return 0, false
	}
	for i, row := range fr.rows {
		if row.kind == rowLine && lineNumberMatches(row.line, c) {
			if c.LineContent == "" || row.line.Content == c.LineContent {
				return i, true
			}
		}
	}
	if c.LineContent == "" {
		return 0, false
	}
	match, matches := -1, 0
	for i, row := range fr.rows {
		if row.kind == rowLine && row.line.Content == c.LineContent {
			match, matches = i, matches+1
		}
	}
	if matches == 1 {
		return match, true
	}
	return 0, false
}

// commentAnchorRowSplit is commentAnchorRow's split-view counterpart: the
// same two-pass (exact line-number+content match, then unique-content
// fallback) logic, but checking both sides of a pairedRow — a comment
// anchors to whichever side (old or new) it was originally created
// against, and either side (or, for a genuine modified pair, potentially
// both) can match.
func commentAnchorRowSplit(fr fileRows, c Comment) (rowIdx int, ok bool) {
	if fr.file.Path != c.File {
		return 0, false
	}
	sideMatches := func(l *Line) bool {
		return l != nil && lineNumberMatches(*l, c) && (c.LineContent == "" || l.Content == c.LineContent)
	}
	for i, row := range fr.splitRows {
		if row.kind == rowLine && (sideMatches(row.left) || sideMatches(row.right)) {
			return i, true
		}
	}
	if c.LineContent == "" {
		return 0, false
	}
	contentMatches := func(l *Line) bool { return l != nil && l.Content == c.LineContent }
	match, matches := -1, 0
	for i, row := range fr.splitRows {
		if row.kind == rowLine && (contentMatches(row.left) || contentMatches(row.right)) {
			match, matches = i, matches+1
		}
	}
	if matches == 1 {
		return match, true
	}
	return 0, false
}

// orphanedCommentCount is how many session comments have no row to attach
// to anywhere in the currently loaded diff (see commentAnchorRow) — shown
// in the header so a silently-detached comment stays visible instead of
// just vanishing from the diff pane while still sitting in the session
// file. Comments with no recorded LineContent (created before re-anchoring
// existed) are never counted — there's nothing to detect orphaning with,
// so they fall back to the original pure line-number behavior instead.
func (m model) orphanedCommentCount() int {
	count := 0
	for _, c := range m.session.Comments {
		if c.LineContent == "" {
			continue
		}
		anchored := false
		for _, fr := range m.files {
			if fr.file.Path != c.File {
				continue
			}
			_, anchored = commentAnchorRow(fr, c)
			break
		}
		if !anchored {
			count++
		}
	}
	return count
}
