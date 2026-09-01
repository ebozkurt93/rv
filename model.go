package main

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeNormal mode = iota
	modeComment
	modeHelp
	modeSearch
	modeConfirmDelete
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

// fileRows is a file's diff flattened into rows, precomputed once per file
// so cursor movement is just index arithmetic.
type fileRows struct {
	file FileDiff
	rows []diffRow
	// numWidth is how many digits wide this file's line-number gutter
	// columns need to be — sized to the file's own largest line number
	// rather than a fixed width, so a 40-line file gets a 2-char gutter
	// instead of always reserving room for 4.
	numWidth int
}

func flattenFile(fd FileDiff) fileRows {
	fr := fileRows{file: fd}
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

	status string
	err    error

	keys Keymap

	// UI display prefs — persisted across sessions, see prefs.go.
	sidebarHidden   bool
	showLineNumbers bool
	wrapLines       bool

	// sessionMTime is the on-disk session's last-seen mtime, used to detect
	// changes made by another process (e.g. an agent running `rv comment
	// reply`) while this TUI instance is sitting open — see pollSessionCmd.
	sessionMTime time.Time
}

func newModel(repoRoot string, diffFiles []FileDiff, session Session, diffSpec []string) model {
	files := make([]fileRows, 0, len(diffFiles))
	for _, fd := range diffFiles {
		files = append(files, flattenFile(fd))
	}
	prefs := loadUIPrefs()
	m := model{
		repoRoot:        repoRoot,
		diffSpec:        diffSpec,
		files:           files,
		session:         session,
		keys:            defaultKeymap(),
		sidebarHidden:   prefs.SidebarHidden,
		showLineNumbers: prefs.ShowLineNumbers,
		wrapLines:       prefs.WrapLines,
	}
	if mt, err := sessionModTime(repoRoot); err == nil {
		m.sessionMTime = mt
	}
	if len(files) > 0 {
		m.lineIndex = firstContentRow(files[0].rows)
	}
	return m
}

// uiPrefs snapshots the model's persisted display prefs, for saving back
// to disk after any of them change.
func (m model) uiPrefs() uiPrefs {
	return uiPrefs{
		SidebarHidden:   m.sidebarHidden,
		ShowLineNumbers: m.showLineNumbers,
		WrapLines:       m.wrapLines,
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

// The cursor must never rest on a rowHunkHeader — you can't comment on a
// header, so stopping there is always a dead end that costs an extra
// keypress. Every path that sets lineIndex funnels through one of these
// three so that invariant holds everywhere, not just after }/{.

// firstContentRow returns the index of the first commentable row in rows,
// or 0 if there isn't one (shouldn't happen — every hunk has at least one
// line — but keeps callers safe regardless).
func firstContentRow(rows []diffRow) int {
	for i, r := range rows {
		if r.kind == rowLine {
			return i
		}
	}
	return 0
}

// lastContentRow is firstContentRow's counterpart.
func lastContentRow(rows []diffRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].kind == rowLine {
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
func nearestContentRow(rows []diffRow, idx int) int {
	if len(rows) == 0 {
		return 0
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if rows[idx].kind == rowLine {
		return idx
	}
	for i := idx + 1; i < len(rows); i++ {
		if rows[i].kind == rowLine {
			return i
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if rows[i].kind == rowLine {
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
	m.lineIndex = nearestContentRow(files[newIndex].rows, m.lineIndex)
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

// commentsForCurrentLine returns comments anchored to the cursor's current
// line (matched by file + whichever of old/new line the comment carries).
func (m model) commentsForCurrentLine() []Comment {
	line, ok := m.currentLine()
	if !ok {
		return nil
	}
	return commentsOnLine(m.session.Comments, m.files[m.fileIndex].file.Path, line)
}

// locateComment finds where c's anchor line lives in the currently loaded
// diff — which file, and which row within that file's flattened rows — so
// the cursor can be moved directly to it (see (*model).jumpToComment).
func (m model) locateComment(c Comment) (fileIdx, rowIdx int, ok bool) {
	for fi, fr := range m.files {
		if fr.file.Path != c.File {
			continue
		}
		for ri, row := range fr.rows {
			if row.kind != rowLine {
				continue
			}
			if len(commentsOnLine([]Comment{c}, fr.file.Path, row.line)) > 0 {
				return fi, ri, true
			}
		}
	}
	return 0, 0, false
}

// commentsOnLine returns comments in comments anchored to line within file.
// A comment matches on new-line number if it has one, otherwise on
// old-line number (mirroring how a comment is created in
// (*model).addCommentUnderCursor).
func commentsOnLine(comments []Comment, file string, line Line) []Comment {
	var out []Comment
	for _, c := range comments {
		if c.File != file {
			continue
		}
		if line.NewLine != nil && c.NewLine != nil && *line.NewLine == *c.NewLine {
			out = append(out, c)
		} else if line.OldLine != nil && c.OldLine != nil && line.NewLine == nil && c.NewLine == nil && *line.OldLine == *c.OldLine {
			out = append(out, c)
		}
	}
	return out
}
