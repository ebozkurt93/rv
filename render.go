package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	sidebarWidth = 34
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
	line := lipgloss.NewStyle().Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", max(1, width-lipgloss.Width(title)-lipgloss.Width(counts)-1)), counts),
	)
	rule := lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", width))
	return lipgloss.JoinVertical(lipgloss.Left, line, rule)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m model) renderSidebar() string {
	var b strings.Builder
	for i, fr := range m.files {
		marker := statusMarker(fr.file.Status)
		n := unresolvedCount(m.session.Comments, fr.file.Path)
		label := fmt.Sprintf("%s %s", marker, fr.file.Path)
		if n > 0 {
			label += styleComment.Render(fmt.Sprintf(" (%d)", n))
		}
		if i == m.fileIndex {
			b.WriteString(styleSelected.Render("▸ " + label))
		} else {
			b.WriteString("  " + label)
		}
		b.WriteString("\n")
	}

	innerW := sidebarWidth - borderOverheadW
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}
	return lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(innerW).
		Height(innerH).
		Render(b.String())
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
	rows := m.currentRows()
	file := m.files[m.fileIndex].file

	var b strings.Builder
	for i, row := range rows {
		if row.kind == rowHunkHeader {
			b.WriteString(styleHunk.Render("@@ " + row.hunkHeader))
			b.WriteString("\n")
			continue
		}

		text := renderLine(row.line)
		if i == m.lineIndex {
			text = styleCursor.Render(text)
		}
		b.WriteString(text)
		b.WriteString("\n")

		for _, c := range commentsOnLine(m.session.Comments, file.Path, row.line) {
			b.WriteString(renderComment(c))
			b.WriteString("\n")
		}

		if m.mode == modeComment && i == m.lineIndex {
			b.WriteString(m.renderCommentEditor())
			b.WriteString("\n")
		}
	}

	innerW := m.width - sidebarWidth - borderOverheadW
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}
	return lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(innerW).
		Height(innerH).
		Render(b.String())
}

// renderCommentEditor is the inline "leave a comment" box shown directly
// beneath the line it will be anchored to, rather than in the footer, so the
// comment stays visually attached to the code it's about.
func (m model) renderCommentEditor() string {
	width := m.width - sidebarWidth - borderOverheadW - 4
	if width < 10 {
		width = 10
	}
	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(colorAccent).
		Padding(0, 1).
		Width(width)
	return style.Render(styleComment.Render("comment: ") + m.input + "█")
}

// bodyHeight is the vertical space left for the sidebar+diff row after the
// header and footer.
func (m model) bodyHeight() int {
	h := m.height - 4 // 2 header lines + 1 footer line + slack
	if h < 3 {
		h = 3
	}
	return h
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
	text := "  ▏💬 " + c.Body
	if c.Resolved {
		return styleResolved.Render(text + " (resolved)")
	}
	return styleComment.Render(text)
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
	return styleMuted.Render("j/k move · tab/[ file · gg/G top/bottom · c comment · d delete · r resolve · e export · q quit")
}
