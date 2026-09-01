package main

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Colors are all drawn from the basic 16-color ANSI palette (as opposed to
// 256-color hex-ish codes), so every hue here is whatever the user's
// terminal profile/theme has configured for that slot — this is what makes
// rv "follow the terminal theme" rather than imposing its own. Plain text
// (context lines, unselected sidebar entries) is left with no color at all,
// so it renders in the terminal's own default foreground.
var (
	colorBorder  = lipgloss.Color("8") // bright black — a theme's usual "subtle" gray
	colorAccent  = lipgloss.Color("6") // cyan — selection/focus accent
	colorAdded   = lipgloss.Color("2") // green
	colorRemoved = lipgloss.Color("1") // red
	colorComment = lipgloss.Color("3") // yellow
	colorMuted   = lipgloss.Color("8")
	colorError   = lipgloss.Color("1")

	styleAdded    = lipgloss.NewStyle().Foreground(colorAdded)
	styleRemoved  = lipgloss.NewStyle().Foreground(colorRemoved)
	styleHunk     = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleSelected = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleComment  = lipgloss.NewStyle().Foreground(colorComment).Italic(true)
	styleResolved = lipgloss.NewStyle().Foreground(colorMuted).Strikethrough(true)
	styleError    = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	styleTitle    = lipgloss.NewStyle().Bold(true)

	panelBorder = lipgloss.RoundedBorder()

	// maxSidebarWidth/minSidebarWidth bound (model).sidebarWidth() — it
	// shrinks to fit the longest file path when that's narrower than the
	// max, rather than always claiming the full width regardless of
	// content.
	maxSidebarWidth = 44
	minSidebarWidth = 24

	// borderOverhead is how much a bordered+padded panel adds beyond its
	// content width/height (1 col/row of border plus 1 col of horizontal
	// padding on each side).
	borderOverheadW = 4
	borderOverheadH = 2
)

func (m model) View() string {
	header := m.renderHeader()

	if m.mode == modeHelp {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.renderHelp(), m.renderFooter())
	}

	if len(m.files) == 0 {
		empty := lipgloss.NewStyle().Padding(2, 4).Render(styleMuted.Render("No changes vs HEAD."))
		return lipgloss.JoinVertical(lipgloss.Left, header, empty, m.renderFooter())
	}

	diff := m.renderDiff()
	body := diff
	if !m.sidebarHidden {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), diff)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, m.renderFooter())
}

// diffPaneSidebarWidth is how much horizontal space the sidebar takes out
// of the diff pane's budget — 0 when hidden, so the diff pane reclaims the
// full width.
func (m model) diffPaneSidebarWidth() int {
	if m.sidebarHidden {
		return 0
	}
	return m.sidebarWidth()
}

// sidebarWidth sizes the sidebar to its longest file path (plus the row's
// fixed prefix/marker/checkmark/spacing overhead and the panel's own
// border+padding), capped at maxSidebarWidth and floored at
// minSidebarWidth — so a diff with only a few short filenames doesn't force
// a sidebar as wide as one with deeply nested paths. Sized off the full
// file list rather than whatever's currently visible under a filter, so
// the layout doesn't resize as you type into the file-name search.
func (m model) sidebarWidth() int {
	longest := 0
	for _, fr := range m.files {
		if l := len([]rune(fr.file.Path)); l > longest {
			longest = l
		}
	}
	const rowOverhead = 5 // prefix(2) + marker(1) + check(1) + space(1)
	want := longest + rowOverhead + borderOverheadW
	if want > maxSidebarWidth {
		want = maxSidebarWidth
	}
	if want < minSidebarWidth {
		want = minSidebarWidth
	}
	return want
}

func (m model) renderHeader() string {
	repo := m.repoRoot
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	total, unresolved := len(m.session.Comments), 0
	for _, c := range m.session.Comments {
		if !c.Resolved {
			unresolved++
		}
	}
	reviewedCount := 0
	for _, fr := range m.files {
		if isFileReviewed(m.session, fr.file) {
			reviewedCount++
		}
	}

	added, removed := m.diffStats()
	stats := styleAdded.Render(fmt.Sprintf("+%d", added)) + " " + styleRemoved.Render(fmt.Sprintf("-%d", removed))
	title := styleTitle.Render("rv") + styleMuted.Render(" · "+repo+" · vs "+m.diffLabel()+" · ") + stats
	countsText := fmt.Sprintf("%d/%d file(s) reviewed, %d comment(s), %d unresolved", reviewedCount, len(m.files), total, unresolved)
	if m.fileFilter != "" {
		shown := len(m.visibleFileIndices(m.fileFilter))
		countsText += fmt.Sprintf(" · filter %q (%d/%d shown)", m.fileFilter, shown, len(m.files))
	}
	counts := styleMuted.Render(countsText)

	width := m.width
	if width < 1 {
		width = 80
	}
	gap := width - lipgloss.Width(title) - lipgloss.Width(counts) - 1
	if gap < 1 {
		gap = 1
	}
	line := lipgloss.NewStyle().MaxWidth(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", gap), counts),
	)
	rule := lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", width))

	if len(m.files) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, line, rule)
	}

	// The sidebar truncates long paths for space; show the selected file's
	// full path here, where a whole terminal width is available for it —
	// plus where the cursor currently is, so jumping around (}/{ especially)
	// always confirms where you landed regardless of whether # is on.
	// fitLine truncates from the tail, so a line-number suffix appended
	// after a long path would just get cut off and never actually be
	// visible — reserve its space first, then truncate only the path
	// (keeping the path's own tail — the filename — via truncateKeepingTail).
	prefix := "▸ "
	selFile := m.files[m.fileIndex].file
	label := ""
	if l := m.currentLineLabel(); l != "" {
		label = " :" + l
	}
	selAdded, selRemoved := fileDiffStats(selFile)
	statsLabel := ""
	if selAdded > 0 || selRemoved > 0 {
		statsLabel = fmt.Sprintf(" +%d -%d", selAdded, selRemoved)
	}

	avail := width - lipgloss.Width(prefix) - lipgloss.Width(label) - lipgloss.Width(statsLabel)
	truncatedPath := truncateKeepingTail(selFile.Path, avail)

	styledLabel := styleMuted.Render(label)
	if statsLabel != "" {
		styledLabel += styleAdded.Render(fmt.Sprintf(" +%d", selAdded)) + styleRemoved.Render(fmt.Sprintf(" -%d", selRemoved))
	}
	path := fitLine(styleSelected.Render(prefix)+truncatedPath+styledLabel, width)
	return lipgloss.JoinVertical(lipgloss.Left, line, rule, path)
}

// bodyHeight is the vertical space left for the sidebar+diff row after the
// header (title + rule + selected-path lines) and footer.
func (m model) bodyHeight() int {
	h := m.height - 5 // 3 header lines + 1 footer line + slack
	if h < 3 {
		h = 3
	}
	return h
}

// fitBlock pins lines to exactly height entries — truncating extras or
// padding with blanks — so a panel's rendered height never depends on how
// much content it happens to have. Without this, a long file or a cursor
// line with several comments under it would make one frame taller than the
// next, and since both panels sit in the same JoinHorizontal row, the whole
// layout (sidebar included) would visibly shift as you scroll.
func fitBlock(lines []string, height int) []string {
	if height < 0 {
		height = 0
	}
	if len(lines) > height {
		return lines[:height]
	}
	out := make([]string, height)
	copy(out, lines)
	return out
}

// fitLine truncates (never wraps) s to exactly width cells, right-padding
// with spaces if it's shorter. Every line handed to a bordered panel goes
// through this, so the panel's rendered width is always exactly width
// regardless of content — critical because lipgloss.Style.Width() wraps
// (rather than truncates) any line that's still too long once its own
// padding is subtracted, which otherwise silently turns one logical line
// into two rendered ones and throws off every height calculation above it.
func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	if w := ansi.StringWidth(s); w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// fitLineWithBackground is fitLine, but bg is applied to the *entire*
// physical line (real text plus any padding), not just the padding — used
// for rows whose text already carries a baked-in chroma background
// (added/removed/cursor tint, see renderLine) so the tint spans the whole
// row instead of stopping wherever the text happens to end.
//
// bg is prepended before s itself, not just appended after for the padding
// shortfall, because word-wrap (wrapLine) can split a single chroma token —
// one open-color/text/reset span — across two physical lines at a
// mid-token word boundary. The continuation fragment on the second physical
// line then starts with no color codes of its own (its token's opening
// escape stayed on the first physical line); s can still be a well-formed,
// self-contained ANSI string, but the color it "resumes into" isn't
// guaranteed active at that fragment's start once each physical line is
// composited independently into the bordered panel. Prepending bg
// (re-)establishes it from column 0 regardless.
//
// bg is emitted as a raw truecolor escape (not via lipgloss.Style) so it
// always matches the tint tintedFormatter baked into the text exactly,
// regardless of what color profile lipgloss's own terminal detection lands
// on — the tint hexes are fixed truecolor values (see syntax.go), not one
// of the basic 16 ANSI colors, so a profile downgrade would otherwise shift
// this to a visibly different color than the text next to it.
func fitLineWithBackground(s string, width int, bg lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	c := chroma.MustParseColour(string(bg))
	bgEscape := fmt.Sprintf("\033[48;2;%d;%d;%dm", c.Red(), c.Green(), c.Blue())
	pad := ""
	if w := ansi.StringWidth(s); w < width {
		// bgEscape again here, not just up front — s's own last token
		// already closed with its own reset (every token is self-contained,
		// see tintedFormatter), which would otherwise leave this padding
		// colorless even though bgEscape was applied at the very start.
		pad = bgEscape + strings.Repeat(" ", width-w)
	}
	return bgEscape + s + pad + "\033[0m"
}

// clampScroll centers cursor within a height-tall window over [0, total),
// clamped so the window never runs past either end.
func clampScroll(cursor, total, height int) int {
	if height <= 0 || total <= height {
		return 0
	}
	scroll := cursor - height/2
	if scroll < 0 {
		scroll = 0
	}
	if max := total - height; scroll > max {
		scroll = max
	}
	return scroll
}

func (m model) renderSidebar() string {
	innerW := m.sidebarWidth() - borderOverheadW
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}

	// While actively typing a filter, show live results for the in-progress
	// query even though it isn't committed to m.fileFilter yet.
	query := m.fileFilter
	if m.mode == modeSearch {
		query = m.input
	}
	vis := m.visibleFileIndices(query)

	lines := make([]string, len(vis))
	cursorPos := 0
	for pos, fi := range vis {
		fr := m.files[fi]
		reviewed := isFileReviewed(m.session, fr.file)
		lines[pos] = renderSidebarRow(fr, fi == m.fileIndex, reviewed, unresolvedCount(m.session.Comments, fr.file.Path), innerW)
		if fi == m.fileIndex {
			cursorPos = pos
		}
	}

	scroll := clampScroll(cursorPos, len(lines), innerH)
	window := fitBlock(lines[scroll:min(scroll+innerH, len(lines))], innerH)
	for i, l := range window {
		window[i] = fitLine(l, innerW)
	}

	return lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Render(strings.Join(window, "\n"))
}

// renderSidebarRow renders a single sidebar line, truncated (never wrapped)
// to fit width — the file path is truncated from the left so the filename
// itself, the most useful part, stays visible.
func renderSidebarRow(fr fileRows, selected, reviewed bool, unresolved int, width int) string {
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	marker := statusMarker(fr.file.Status)
	check := " "
	if reviewed {
		check = styleAdded.Render("✓")
	}

	unresolvedText := ""
	if unresolved > 0 {
		unresolvedText = fmt.Sprintf(" (%d)", unresolved)
	}

	avail := width - lipgloss.Width(prefix) - lipgloss.Width(marker) - lipgloss.Width(check) - 1 - lipgloss.Width(unresolvedText)
	path := truncateKeepingTail(fr.file.Path, avail)

	styledSuffix := styleComment.Render(unresolvedText)

	// Concatenate independently-rendered segments rather than nesting one
	// Style.Render call inside another — nesting would let the inner
	// segment's reset code cut off the outer style partway through the
	// line. Reviewed dims the path (muted) regardless of selection, to
	// visually de-emphasize files you've already gone through.
	if selected {
		pathPart := styleSelected.Render(" " + path)
		if reviewed {
			pathPart = styleMuted.Render(" " + path)
		}
		return styleSelected.Render(prefix) + marker + check + pathPart + styledSuffix
	}
	pathPart := path
	if reviewed {
		pathPart = styleMuted.Render(path)
	}
	return prefix + marker + check + " " + pathPart + styledSuffix
}

// truncateKeepingTail truncates s to at most width cells, preferring to
// drop characters from the front (prefixed with "…") so the tail — for a
// file path, the filename — survives.
func truncateKeepingTail(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	r := []rune(s)
	keep := width - 1
	if keep > len(r) {
		keep = len(r)
	}
	return "…" + string(r[len(r)-keep:])
}

func statusMarker(s FileStatus) string {
	switch s {
	case FileAdded:
		return styleAdded.Render("A")
	case FileDeleted:
		return styleRemoved.Render("D")
	default:
		return styleHunk.Render("M")
	}
}

func unresolvedCount(comments []Comment, file string) int {
	n := 0
	for _, c := range comments {
		if c.File == file && !c.Resolved {
			n++
		}
	}
	return n
}

func (m model) renderDiff() string {
	innerW := m.width - m.diffPaneSidebarWidth() - borderOverheadW
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}

	lines, cursorLine, rowFor, mainLine := m.buildDiffLinesDetailed(innerW)
	scroll := clampScroll(cursorLine, len(lines), innerH)
	end := min(scroll+innerH, len(lines))
	window := fitBlock(lines[scroll:end], innerH)

	rows := m.currentRows()
	for i := range window {
		idx := scroll + i
		bg, tinted := rowBackground(rows, rowFor, mainLine, idx, m.lineIndex)
		if tinted {
			window[i] = fitLineWithBackground(window[i], innerW, bg)
		} else {
			window[i] = fitLine(window[i], innerW)
		}
	}

	return renderBorderedRaw(innerW, window)
}

// renderBorderedRaw draws the same rounded-border-plus-1-space-padding box
// lipgloss.NewStyle().Border(...).Padding(0,1).Render(...) would, but
// without ever handing already-colored content lines to lipgloss's own
// Render — lipgloss's layout engine reprocesses embedded ANSI when it
// computes per-line padding/width, which rewrites a full reset (\033[0m)
// into separate default-fg/default-bg codes and then pads any shortfall
// with its own plain (uncolored) spaces — silently destroying the
// truecolor background fitLineWithBackground already baked all the way
// across each line. Border corners/edges are rendered independently and
// concatenated (never nested around a multi-segment already-rendered
// string), same rule as everywhere else raw ANSI content gets composed.
func renderBorderedRaw(innerW int, lines []string) string {
	borderStyle := lipgloss.NewStyle().Foreground(colorBorder)
	top := borderStyle.Render(panelBorder.TopLeft + strings.Repeat(panelBorder.Top, innerW+2) + panelBorder.TopRight)
	bottom := borderStyle.Render(panelBorder.BottomLeft + strings.Repeat(panelBorder.Bottom, innerW+2) + panelBorder.BottomRight)
	left := borderStyle.Render(panelBorder.Left)
	right := borderStyle.Render(panelBorder.Right)

	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, top)
	for _, l := range lines {
		rows = append(rows, left+" "+l+" "+right)
	}
	rows = append(rows, bottom)
	return strings.Join(rows, "\n")
}

// rowBackground decides whether the rendered line at lines[idx] (idx may be
// out of range — fitBlock pads window past the real content with "") needs
// its trailing padding tinted to match the background renderLine already
// baked into that row's syntax-highlighted text, and if so, which color.
// The cursor row's tint takes priority over its diff-status tint, matching
// renderLine's own precedence.
func rowBackground(rows []diffRow, rowFor []int, mainLine []bool, idx, cursorRowIdx int) (lipgloss.Color, bool) {
	if idx < 0 || idx >= len(rowFor) {
		return "", false
	}
	if !mainLine[idx] {
		return "", false
	}
	origin := rowFor[idx]
	if origin < 0 || origin >= len(rows) {
		return "", false
	}
	if origin == cursorRowIdx {
		return bgCursor, true
	}
	row := rows[origin]
	if row.kind != rowLine {
		return "", false
	}
	switch row.line.Kind {
	case LineAdded:
		return bgAdded, true
	case LineRemoved:
		return bgRemoved, true
	default:
		return "", false
	}
}

// buildDiffLines flattens the current file's rows (plus any comments and,
// if active, the inline comment editor) into individually-addressable
// render lines, and reports which line the cursor is "on" so the caller can
// scroll to keep it in view. width is the diff pane's content width — only
// used when m.wrapLines is on (see appendText below); ignored otherwise,
// since renderDiff's later fitLine pass handles plain truncation.
//
// rowFor is lines' parallel index: rowFor[k] is the currentRows() index
// that produced lines[k] — every wrapped sub-line, and every comment/reply/
// editor line attached to a row, maps back to that same row. Used by
// mouse.go to turn a click's screen Y back into a row to focus, replaying
// this exact same construction (including scroll) rather than assuming one
// rendered line always equals one row, which wrap mode breaks.
func (m model) buildDiffLines(width int) (lines []string, cursorLine int, rowFor []int) {
	l, c, r, _ := m.buildDiffLinesDetailed(width)
	return l, c, r
}

// buildDiffLinesDetailed is buildDiffLines plus a mainLine marker: mainLine[k]
// is true only for a row's own diff-line/hunk-header rendering (including its
// wrap continuations), false for comment/reply/editor lines attached beneath
// it — so callers that tint a row's background (renderDiff) don't bleed that
// tint onto a comment's padding, which never had it baked into its own text.
func (m model) buildDiffLinesDetailed(width int) (lines []string, cursorLine int, rowFor []int, mainLine []bool) {
	rows := m.currentRows()
	file := m.files[m.fileIndex].file

	appendText := func(text string, rowIdx int, main bool) {
		if m.wrapLines {
			for _, l := range wrapLine(text, width) {
				lines = append(lines, l)
				rowFor = append(rowFor, rowIdx)
				mainLine = append(mainLine, main)
			}
			return
		}
		lines = append(lines, text)
		rowFor = append(rowFor, rowIdx)
		mainLine = append(mainLine, main)
	}

	for i, row := range rows {
		if row.kind == rowHunkHeader {
			start := len(lines)
			appendText(styleHunk.Render("@@ "+row.hunkHeader), i, false)
			if i == m.lineIndex {
				cursorLine = start
			}
			continue
		}

		cursor := i == m.lineIndex
		text := renderLine(m.files[m.fileIndex].lexer, row.line, m.showLineNumbers, m.files[m.fileIndex].numWidth, cursor)
		start := len(lines)
		if cursor {
			cursorLine = start
		}
		appendText(text, i, true)

		for _, c := range commentsOnLine(m.session.Comments, file.Path, row.line) {
			for _, l := range strings.Split(renderComment(c), "\n") {
				appendText(l, i, false)
			}
			for _, r := range c.Replies {
				for _, l := range strings.Split(renderReply(r), "\n") {
					appendText(l, i, false)
				}
			}
		}
		if m.mode == modeComment && i == m.lineIndex {
			for _, l := range strings.Split(m.renderCommentEditor(), "\n") {
				lines = append(lines, l)
				rowFor = append(rowFor, i)
				mainLine = append(mainLine, false)
			}
		}
	}
	return lines, cursorLine, rowFor, mainLine
}

// wrapLine word-wraps s (which may already carry ANSI styling — lipgloss's
// underlying wrap implementation is ANSI-aware and preserves it across the
// break) to width cells, returning one entry per resulting physical line.
// Used only when wrapLines is on; the default (off) truncates instead, via
// fitLine in renderDiff, to keep every row exactly one line tall.
//
// Style.Width() doesn't just wrap — it also right-pads every resulting
// physical line to exactly width with plain, unstyled spaces, since it has
// no idea our content carries a tint that padding should match. Each
// returned line is trimmed back down to its real content so renderDiff's
// fitLine/fitLineWithBackground (which DO know about the tint) are the ones
// deciding what the padding looks like — otherwise a tinted row that wraps
// arrives at fitLineWithBackground already at full width, so its own
// padding branch never runs, and the wrap-added padding stays uncolored.
func wrapLine(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	rawLines := strings.Split(lipgloss.NewStyle().Width(width).Render(s), "\n")
	lines := make([]string, len(rawLines))
	for i, l := range rawLines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

// helpRow is one line of the ? overlay: either a section header
// (Section != "", everything else zero) or a keybinding (Keys resolved live
// from the actual Keymap — including any config-file remaps — paired with
// a static description).
type helpRow struct {
	section string
	keys    []string
	desc    string
}

// helpRows is generated from m.keys (not a hardcoded list of default key
// names) specifically so a remapped binding — via the config file, see
// config.go — shows its real key here instead of a stale default.
func (m model) helpRows() []helpRow {
	k := m.keys
	return []helpRow{
		{section: "Movement"},
		{keys: k.MoveDown, desc: "move down (a count prefix repeats it, e.g. 10×)"},
		{keys: k.MoveUp, desc: "move up"},
		{keys: k.HalfPageDown, desc: "half page down"},
		{keys: k.HalfPageUp, desc: "half page up"},
		{keys: k.Top, desc: "jump to top (press twice); a count jumps to that line"},
		{keys: k.Bottom, desc: "jump to bottom; a count jumps to that line"},
		{keys: k.NextHunk, desc: "next hunk (current file only)"},
		{keys: k.PrevHunk, desc: "prev hunk (current file only)"},
		{keys: k.NextComment, desc: "next unresolved comment (across files, wraps)"},
		{keys: k.PrevComment, desc: "prev unresolved comment (across files, wraps)"},
		{keys: k.NextFile, desc: "next file"},
		{keys: k.PrevFile, desc: "prev file"},
		{keys: k.Search, desc: "filter files by name (enter confirms, esc cancels)"},
		{section: "Comments"},
		{keys: k.AddComment, desc: "add a comment on the line under the cursor"},
		{keys: k.CommentNewline, desc: "while composing: insert a newline instead of submitting"},
		{keys: k.CommentEditor, desc: "while composing: edit the comment body in $EDITOR"},
		{keys: k.DeleteComment, desc: "delete the comment under the cursor (asks to confirm)"},
		{keys: k.ToggleResolved, desc: "toggle resolved"},
		{keys: k.ToggleReviewed, desc: "mark/unmark the current file reviewed (auto-clears if it changes)"},
		{section: "Display"},
		{keys: k.ToggleSidebar, desc: "show/hide the sidebar"},
		{keys: k.ToggleLineNumbers, desc: "show/hide line numbers"},
		{keys: k.ToggleWrap, desc: "wrap long lines instead of truncating"},
		{section: "Mouse"},
		{keys: []string{"click"}, desc: "click a sidebar row to select that file"},
		{keys: []string{"scroll"}, desc: "over the sidebar: change file · over the diff: move the cursor"},
		{section: "Other"},
		{keys: k.OpenEditor, desc: "open the file under the cursor in $EDITOR"},
		{keys: k.Refresh, desc: "refresh (re-read the diff and session)"},
		{keys: k.Export, desc: "export the review to a markdown file"},
		{keys: k.CopyMarkdown, desc: "copy the review (markdown) to the clipboard"},
		{keys: k.CopyJSON, desc: "copy the review (json) to the clipboard"},
		{keys: k.Help, desc: "toggle this help"},
		{keys: k.Quit, desc: "quit"},
	}
}

// helpLines flattens helpRows() into one rendered string per line (section
// headers get a blank-line separator above them), shared by renderHelp (to
// display a scrolled window of them) and updateHelp (to clamp how far that
// window can scroll) so the two can never disagree about the total count.
func (m model) helpLines() []string {
	var lines []string
	for _, row := range m.helpRows() {
		if row.section != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, styleTitle.Render(row.section))
			continue
		}
		lines = append(lines, fmt.Sprintf("  %-18s %s", strings.Join(row.keys, " / "), row.desc))
	}
	return lines
}

func (m model) renderHelp() string {
	innerW := m.width - borderOverheadW
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}

	lines := m.helpLines()
	scroll := m.helpScroll
	if max := len(lines) - innerH; scroll > max {
		scroll = max
	}
	if scroll < 0 {
		scroll = 0
	}
	window := fitBlock(lines[scroll:min(scroll+innerH, len(lines))], innerH)
	for i, l := range window {
		window[i] = fitLine(l, innerW)
	}

	return lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Render(strings.Join(window, "\n"))
}

// renderCommentEditor is the inline "leave a comment" row(s) shown directly
// beneath the line it will be anchored to, rather than in the footer, so the
// comment stays visually attached to the code it's about. Styled to match
// renderComment/renderReply (same indent/prefix convention, no border) —
// it's a comment that just hasn't been submitted yet, not a modal dialog.
// m.input can span multiple lines (alt+enter inserts one, see updateComment)
// — every line but the last is rendered in full; only the in-progress last
// line gets the "keep the tail visible, not the start" truncation and the
// cursor block, since that's the one still being typed into.
func (m model) renderCommentEditor() string {
	width := m.width - m.diffPaneSidebarWidth() - borderOverheadW
	if width < 10 {
		width = 10
	}
	// The icon distinguishes what submitting will do — matches
	// commentActionForCurrentLine's own decision (see AddComment in
	// updateNormal), so what's about to happen is visible while typing.
	icon := "✎" // editing in place, or a brand new comment
	if m.replyingToCommentID != "" {
		icon = "↩" // appending a new reply to an existing thread
	}
	firstPrefix, contPrefix := "  ▏"+icon+" ", "     "
	inputLines := strings.Split(m.input, "\n")
	rendered := make([]string, len(inputLines))
	for i, l := range inputLines {
		prefix := contPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		if i == len(inputLines)-1 {
			l = truncateKeepingTail(l, width-lipgloss.Width(prefix)-1) + "█"
		}
		rendered[i] = styleComment.Render(prefix + l)
	}
	return strings.Join(rendered, "\n")
}

// renderLine renders one diff line's gutter + prefix + syntax-highlighted
// content. cursor picks the cursor-tinted syntax style regardless of diff
// status (matching how a "current line" highlight in most editors takes
// priority over other line-level decoration); otherwise the style follows
// l.Kind (added/removed/context). The +/- prefix character is rendered
// separately from the highlighted content (own top-level Render call,
// concatenated rather than nested — see tintedStyle's doc comment for why
// that distinction matters here).
func renderLine(lexer chroma.Lexer, l Line, showNumbers bool, numWidth int, cursor bool) string {
	prefix := " "
	prefixFg := lipgloss.Color("")
	fmtr := syntaxContextFmt
	bg := lipgloss.Color("")
	switch l.Kind {
	case LineAdded:
		prefix = "+"
		prefixFg = colorAdded
		bg = bgAdded
		fmtr = syntaxAddedFmt
	case LineRemoved:
		prefix = "-"
		prefixFg = colorRemoved
		bg = bgRemoved
		fmtr = syntaxRemovedFmt
	}
	if cursor {
		fmtr = syntaxCursorFmt
		bg = bgCursor
	}

	// Built as raw escapes (not lipgloss.Style.Background) whenever a tint
	// applies, so the prefix/gutter's background always matches the
	// syntax-highlighted content's truecolor tint exactly — see
	// tintBgEscape's doc comment for why a lipgloss-rendered segment can't
	// be trusted to land on the same shade.
	renderTinted := func(text string, fg lipgloss.Color) string {
		if bg == "" {
			if fg == "" {
				return lipgloss.NewStyle().Render(text)
			}
			return lipgloss.NewStyle().Foreground(fg).Render(text)
		}
		return ansiFgEscape(fg) + tintBgEscape(bg) + text + "\033[0m"
	}

	// Tabs are expanded to a fixed number of spaces before any of the
	// width-sensitive rendering below — ansi.StringWidth/ansi.Truncate (used
	// throughout this file for column-exact layout) count a literal tab as
	// either 0 or 1 cell, while a real terminal jumps it to the next
	// tab-stop (up to 8 columns). That mismatch under-counts the line's true
	// rendered width, throwing off truncation/padding and, with
	// renderBorderedRaw's fixed-width row assembly, visibly misaligning the
	// right border on any line that came from tab-indented source (i.e. most
	// Go source lines). l.Content itself is left untouched — this only
	// affects what's displayed, not comment anchoring/hashing/export.
	visibleContent := strings.ReplaceAll(l.Content, "\t", "    ")
	content := renderTinted(prefix, prefixFg) + highlightContent(lexer, fmtr, visibleContent)
	if !showNumbers {
		return content
	}
	// The gutter's numbers share the prefix's exact color (muted for
	// context, green/red for added/removed) rather than always being muted
	// — GitHub/Hunk color the gutter the same as the +/- marker, and it
	// doubles as the contrast-safe choice against the tinted background
	// (green/red already reads fine there; it's what's already used on the
	// tint everywhere else on the row).
	gutterFg := colorMuted
	if l.Kind != LineContext {
		gutterFg = prefixFg
	}
	// Independently rendered and concatenated, not nested inside content's
	// own Render call — nesting would let content's reset code cut the
	// gutter's styling off partway through. No literal separator character
	// — the muted color plus the trailing space are enough to read as
	// "gutter, then code" without adding visual noise. renderTinted carries
	// the same background tint as the content/prefix so a tinted row's
	// background covers the numbers too, not just the code.
	gutter := renderTinted(fmt.Sprintf("%*s %*s  ", numWidth, lineNumStr(l.OldLine), numWidth, lineNumStr(l.NewLine)), gutterFg)
	return gutter + content
}

func lineNumStr(n *int) string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%d", *n)
}

// renderComment/renderReply's own trailing "\n" between the first line and
// any continuation lines is intentional — the caller (buildDiffLinesDetailed)
// splits back on "\n" into separate rendered rows, exactly like
// renderCommentEditor does for a multi-line comment still being typed.
func renderComment(c Comment) string {
	text := commentBodyLines("  ▏💬 "+c.Author+": ", "     ", c.Body)
	if c.Resolved {
		return styleResolved.Render(text + " (resolved)")
	}
	return styleComment.Render(text)
}

func renderReply(r Reply) string {
	text := commentBodyLines("    └─ "+r.Author+": ", "       ", r.Body)
	return styleMuted.Render(text)
}

// commentBodyLines joins body's lines with firstPrefix on the first and
// contPrefix (aligned under it) on every continuation line.
func commentBodyLines(firstPrefix, contPrefix, body string) string {
	lines := strings.Split(body, "\n")
	text := firstPrefix + lines[0]
	for _, cont := range lines[1:] {
		text += "\n" + contPrefix + cont
	}
	return text
}

// firstKey returns keys' primary binding, for compact footer hints — the
// full list (all bound keys) is what the ? overlay shows.
func firstKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func (m model) renderFooter() string {
	if m.mode == modeComment {
		return styleMuted.Render("enter save · esc cancel")
	}
	if m.mode == modeSearch {
		return "/" + m.input + "█"
	}
	if m.mode == modeConfirmDelete {
		return styleError.Render("delete this comment? y/n")
	}
	if m.err != nil {
		return styleError.Render("error: " + m.err.Error())
	}
	if m.countBuffer != "" {
		return styleSelected.Render(m.countBuffer)
	}
	if m.status != "" {
		return styleMuted.Render(m.status)
	}
	k := m.keys
	hint := fmt.Sprintf("%s/%s move · %s comment · %s edit · %s quit · %s help",
		firstKey(k.MoveDown), firstKey(k.MoveUp), firstKey(k.AddComment),
		firstKey(k.OpenEditor), firstKey(k.Quit), firstKey(k.Help))
	return styleMuted.Render(hint)
}
