package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.mode == modeComment {
			return m.updateComment(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	wasPendingG := m.pendingG
	m.pendingG = false
	m.status = ""

	switch {
	case keyMatches(msg, k.Quit):
		return m, tea.Quit

	case keyMatches(msg, k.MoveDown):
		m.moveCursor(1)
	case keyMatches(msg, k.MoveUp):
		m.moveCursor(-1)

	case keyMatches(msg, k.NextFile):
		m.selectFile(m.fileIndex + 1)
	case keyMatches(msg, k.PrevFile):
		m.selectFile(m.fileIndex - 1)

	case keyMatches(msg, k.Bottom):
		rows := m.currentRows()
		if len(rows) > 0 {
			m.lineIndex = len(rows) - 1
		}
	case keyMatches(msg, k.Top):
		if wasPendingG {
			m.lineIndex = 0
		} else {
			m.pendingG = true
		}

	case keyMatches(msg, k.AddComment):
		if _, ok := m.currentLine(); ok {
			m.mode = modeComment
			m.input = ""
		}

	case keyMatches(msg, k.DeleteComment):
		m.deleteCommentUnderCursor()

	case keyMatches(msg, k.ToggleResolved):
		m.toggleResolvedUnderCursor()

	case keyMatches(msg, k.Export):
		if path, err := exportSession(m.repoRoot, m.session, m.diffFiles(), formatMarkdown); err != nil {
			m.err = err
		} else {
			m.status = "exported to " + path
		}
	}

	return m, nil
}

func (m model) updateComment(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case keyMatches(msg, k.Cancel):
		m.mode = modeNormal
		m.input = ""
	case keyMatches(msg, k.Confirm):
		m.addCommentUnderCursor()
		m.mode = modeNormal
		m.input = ""
	case keyMatches(msg, k.Backspace):
		if len(m.input) > 0 {
			r := []rune(m.input)
			m.input = string(r[:len(r)-1])
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.input += string(msg.Runes)
		} else if msg.Type == tea.KeySpace {
			m.input += " "
		}
	}
	return m, nil
}

func (m *model) moveCursor(delta int) {
	rows := m.currentRows()
	if len(rows) == 0 {
		return
	}
	m.lineIndex += delta
	if m.lineIndex < 0 {
		m.lineIndex = 0
	}
	if m.lineIndex >= len(rows) {
		m.lineIndex = len(rows) - 1
	}
}

func (m *model) selectFile(idx int) {
	if len(m.files) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.files) {
		idx = len(m.files) - 1
	}
	m.fileIndex = idx
	m.lineIndex = 0
	m.scroll = 0
}

func (m *model) addCommentUnderCursor() {
	line, ok := m.currentLine()
	if !ok || m.input == "" {
		return
	}
	file := m.files[m.fileIndex].file.Path

	comment := Comment{
		ID:        newCommentID(),
		File:      file,
		OldLine:   line.OldLine,
		NewLine:   line.NewLine,
		Body:      m.input,
		Author:    "user",
		CreatedAt: time.Now(),
	}

	m.mutateSession(func(s Session) (Session, error) {
		s.Comments = append(s.Comments, comment)
		return s, nil
	})
}

func (m *model) deleteCommentUnderCursor() {
	comments := m.commentsForCurrentLine()
	if len(comments) == 0 {
		return
	}
	id := comments[len(comments)-1].ID

	m.mutateSession(func(s Session) (Session, error) {
		s.Comments = removeComment(s.Comments, id)
		return s, nil
	})
}

func (m *model) toggleResolvedUnderCursor() {
	comments := m.commentsForCurrentLine()
	if len(comments) == 0 {
		return
	}
	id := comments[len(comments)-1].ID

	m.mutateSession(func(s Session) (Session, error) {
		toggleResolved(s.Comments, id)
		return s, nil
	})
}

// mutateSession runs fn under the on-disk session lock via withSession, then
// reloads m.session from disk so the in-memory copy can never drift from
// what's persisted.
func (m *model) mutateSession(fn func(Session) (Session, error)) {
	if err := withSession(m.repoRoot, fn); err != nil {
		m.err = err
		return
	}
	s, err := loadSession(m.repoRoot)
	if err != nil {
		m.err = err
		return
	}
	m.session = s
}

func removeComment(comments []Comment, id string) []Comment {
	out := comments[:0]
	for _, c := range comments {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return out
}

func toggleResolved(comments []Comment, id string) {
	for i := range comments {
		if comments[i].ID == id {
			comments[i].Resolved = !comments[i].Resolved
			return
		}
	}
}

// diffFiles reconstructs the []FileDiff the model was built from, for
// export.
func (m model) diffFiles() []FileDiff {
	out := make([]FileDiff, 0, len(m.files))
	for _, fr := range m.files {
		out = append(out, fr.file)
	}
	return out
}
