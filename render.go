package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleAdded     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleRemoved   = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	styleContext   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleHunk      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleCursor    = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	styleSidebar   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	styleSelected  = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	styleComment   = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Italic(true)
	styleResolved  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	styleStatusBar = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	sidebarWidth   = 32
)

func (m model) View() string {
	if len(m.files) == 0 {
		return "rv: no changes vs HEAD\n\n(press q to quit)\n"
	}

	sidebar := m.renderSidebar()
	diff := m.renderDiff()
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, diff)

	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m model) renderSidebar() string {
	var b strings.Builder
	for i, fr := range m.files {
		marker := statusMarker(fr.file.Status)
		n := unresolvedCount(m.session.Comments, fr.file.Path)
		label := fmt.Sprintf("%s %s", marker, fr.file.Path)
		if n > 0 {
			label += fmt.Sprintf(" (%d)", n)
		}
		if i == m.fileIndex {
			b.WriteString(styleSelected.Render("▸ " + label))
		} else {
			b.WriteString(styleSidebar.Render("  " + label))
		}
		b.WriteString("\n")
	}
	height := m.height - 1
	if height < 1 {
		height = 1
	}
	return lipgloss.NewStyle().Width(sidebarWidth).Height(height).Padding(0, 1).Render(b.String())
}

func statusMarker(s FileStatus) string {
	switch s {
	case FileAdded:
		return "A"
	case FileDeleted:
		return "D"
	default:
		return "M"
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
	}

	width := m.width - sidebarWidth
	if width < 1 {
		width = 1
	}
	height := m.height - 1
	if height < 1 {
		height = 1
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func renderLine(l Line) string {
	prefix := " "
	style := styleContext
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
	text := "    💬 " + c.Body
	if c.Resolved {
		return styleResolved.Render(text + " (resolved)")
	}
	return styleComment.Render(text)
}

func (m model) renderFooter() string {
	if m.mode == modeComment {
		return styleStatusBar.Render("comment> " + m.input + "█")
	}
	if m.err != nil {
		return styleError.Render("error: " + m.err.Error())
	}
	if m.status != "" {
		return styleStatusBar.Render(m.status)
	}
	return styleStatusBar.Render("j/k move  tab/[ file  gg/G top/bottom  c comment  d delete  r resolve  e export  q quit")
}
