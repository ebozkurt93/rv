package main

import (
	"fmt"
	"strings"

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
	styleCursor   = lipgloss.NewStyle().Reverse(true)
	styleSelected = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleComment  = lipgloss.NewStyle().Foreground(colorComment).Italic(true)
	styleResolved = lipgloss.NewStyle().Foreground(colorMuted).Strikethrough(true)
	styleError    = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	styleTitle    = lipgloss.NewStyle().Bold(true)

	panelBorder = lipgloss.RoundedBorder()

	sidebarWidth = 44
	// borderOverhead is how much a bordered+padded panel adds beyond its
	// content width/height (1 col/row of border plus 1 col of horizontal
	// padding on each side).
	borderOverheadW = 4
	borderOverheadH = 2
)

func (m model) View() string {
	header := m.renderHeader()

	if len(m.files) == 0 {
		empty := lipgloss.NewStyle().Padding(2, 4).Render(styleMuted.Render("No changes vs HEAD."))
		return lipgloss.JoinVertical(lipgloss.Left, header, empty, m.renderFooter())
	}

	sidebar := m.renderSidebar()
	diff := m.renderDiff()
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, diff)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, m.renderFooter())
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
	title := styleTitle.Render("rv") + styleMuted.Render(" · "+repo)
	counts := styleMuted.Render(fmt.Sprintf("%d file(s), %d comment(s), %d unresolved", len(m.files), total, unresolved))

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
	// full path here, where a whole terminal width is available for it.
	selected := m.files[m.fileIndex].file.Path
	path := fitLine(styleSelected.Render("▸ ")+selected, width)
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
	innerW := sidebarWidth - borderOverheadW
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}

	lines := make([]string, len(m.files))
	for i, fr := range m.files {
		lines[i] = renderSidebarRow(fr, i == m.fileIndex, unresolvedCount(m.session.Comments, fr.file.Path), innerW)
	}

	scroll := clampScroll(m.fileIndex, len(lines), innerH)
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
func renderSidebarRow(fr fileRows, selected bool, unresolved int, width int) string {
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	marker := statusMarker(fr.file.Status)
	suffix := ""
	if unresolved > 0 {
		suffix = fmt.Sprintf(" (%d)", unresolved)
	}

	avail := width - lipgloss.Width(prefix) - lipgloss.Width(marker) - 1 - lipgloss.Width(suffix)
	path := truncateKeepingTail(fr.file.Path, avail)

	// Concatenate independently-rendered segments rather than nesting one
	// Style.Render call inside another — nesting would let the inner
	// segment's reset code cut off the outer style partway through the line.
	if selected {
		return styleSelected.Render(prefix) + marker + styleSelected.Render(" "+path) + styleComment.Render(suffix)
	}
	return prefix + marker + " " + path + styleComment.Render(suffix)
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
	innerW := m.width - sidebarWidth - borderOverheadW
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}

	lines, cursorLine := m.buildDiffLines()
	scroll := clampScroll(cursorLine, len(lines), innerH)
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

// buildDiffLines flattens the current file's rows (plus any comments and,
// if active, the inline comment editor) into individually-addressable
// render lines, and reports which line the cursor is "on" so the caller can
// scroll to keep it in view.
func (m model) buildDiffLines() (lines []string, cursorLine int) {
	rows := m.currentRows()
	file := m.files[m.fileIndex].file

	for i, row := range rows {
		if row.kind == rowHunkHeader {
			lines = append(lines, styleHunk.Render("@@ "+row.hunkHeader))
			continue
		}

		text := renderLine(row.line)
		if i == m.lineIndex {
			cursorLine = len(lines)
			text = styleCursor.Render(text)
		}
		lines = append(lines, text)

		for _, c := range commentsOnLine(m.session.Comments, file.Path, row.line) {
			lines = append(lines, renderComment(c))
			for _, r := range c.Replies {
				lines = append(lines, renderReply(r))
			}
		}
		if m.mode == modeComment && i == m.lineIndex {
			lines = append(lines, strings.Split(m.renderCommentEditor(), "\n")...)
		}
	}
	return lines, cursorLine
}

// renderCommentEditor is the inline "leave a comment" box shown directly
// beneath the line it will be anchored to, rather than in the footer, so the
// comment stays visually attached to the code it's about.
func (m model) renderCommentEditor() string {
	width := m.width - sidebarWidth - borderOverheadW - 4
	if width < 10 {
		width = 10
	}
	label := "comment: "
	// Keep the tail of what's typed visible (not the start) as input grows
	// past the box — and truncate before rendering, not after, since Width()
	// would otherwise wrap rather than clip an overlong line.
	input := truncateKeepingTail(m.input, width-2-lipgloss.Width(label)-1)
	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(colorAccent).
		Padding(0, 1)
	return style.Render(styleComment.Render(label) + input + "█")
}

func renderLine(l Line) string {
	prefix := " "
	style := lipgloss.NewStyle()
	switch l.Kind {
	case LineAdded:
		prefix = "+"
		style = styleAdded
	case LineRemoved:
		prefix = "-"
		style = styleRemoved
	}
	return style.Render(prefix + l.Content)
}

func renderComment(c Comment) string {
	text := "  ▏💬 " + c.Author + ": " + c.Body
	if c.Resolved {
		return styleResolved.Render(text + " (resolved)")
	}
	return styleComment.Render(text)
}

func renderReply(r Reply) string {
	return styleMuted.Render("    └─ " + r.Author + ": " + r.Body)
}

func (m model) renderFooter() string {
	if m.mode == modeComment {
		return styleMuted.Render("enter save · esc cancel")
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
	return styleMuted.Render("j/k move · tab/[ file · gg/G top/bottom · c comment · d delete · r resolve · R refresh · e export · y/Y copy · q quit")
}
