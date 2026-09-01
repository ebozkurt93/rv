package main

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// sessionPollInterval trades off staleness against needless disk stats;
// an agent replying to a comment shows up within this long without the
// user having to close and reopen the TUI.
const sessionPollInterval = time.Second

type sessionPollMsg struct{}

func pollSessionCmd() tea.Cmd {
	return tea.Tick(sessionPollInterval, func(time.Time) tea.Msg { return sessionPollMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case sessionPollMsg:
		m.refreshSessionIfChanged()
		return m, pollSessionCmd()
	case tea.KeyMsg:
		if m.mode == modeComment {
			return m.updateComment(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

// refreshAll re-reads both the diff (the working tree may have changed —
// e.g. an agent editing files while this TUI sits open — which the
// background session poll never covers, since it only watches
// comments/replies) and the session, without restarting the program.
func (m *model) refreshAll() {
	diffFiles, err := loadDiffFiles()
	if err != nil {
		m.err = err
		return
	}
	m.setDiffFiles(diffFiles)
	m.refreshSessionIfChanged()
	m.status = "refreshed"
}

// refreshSessionIfChanged reloads the on-disk session if its mtime has
// moved since we last read it — i.e. some other process (typically an
// agent running `rv comment reply`/`resolve`) has written to it since.
func (m *model) refreshSessionIfChanged() {
	mt, err := sessionModTime(m.repoRoot)
	if err != nil || !mt.After(m.sessionMTime) {
		return
	}
	s, err := loadSession(m.repoRoot)
	if err != nil {
		return
	}
	m.session = s
	m.sessionMTime = mt
	m.status = "session updated"
}

// digitRune reports whether r can extend a pending vim-style count, and the
// digit it represents. Leading zero doesn't start a count (vim reserves
// bare "0" for move-to-start-of-line), but "10" is fine once "1" has
// already been typed.
func digitRune(r rune, bufferSoFar string) (int, bool) {
	if r < '0' || r > '9' {
		return 0, false
	}
	if r == '0' && bufferSoFar == "" {
		return 0, false
	}
	return int(r - '0'), true
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys

	// Accumulate digits for a pending count (e.g. the "10" of "10j") without
	// touching pendingG, so a count can still precede a "gg" chord.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		if _, ok := digitRune(msg.Runes[0], m.countBuffer); ok {
			m.countBuffer += string(msg.Runes[0])
			m.status = ""
			return m, nil
		}
	}

	hadCount := m.countBuffer != ""
	count := 1
	if hadCount {
		if n, err := strconv.Atoi(m.countBuffer); err == nil && n > 0 {
			count = n
		}
	}

	wasPendingG := m.pendingG
	m.pendingG = false
	m.status = ""
	m.err = nil

	// "g" starts (or, if pendingG, completes) the "gg" chord — don't consume
	// the count buffer on the first "g" or a count typed before it would be
	// lost by the time the second "g" arrives.
	if keyMatches(msg, k.Top) && !wasPendingG {
		m.pendingG = true
		return m, nil
	}
	m.countBuffer = ""

	switch {
	case keyMatches(msg, k.Quit):
		return m, tea.Quit

	case keyMatches(msg, k.MoveDown):
		m.moveCursor(count)
	case keyMatches(msg, k.MoveUp):
		m.moveCursor(-count)

	case keyMatches(msg, k.HalfPageDown):
		m.moveCursor(m.halfPageSize())
	case keyMatches(msg, k.HalfPageUp):
		m.moveCursor(-m.halfPageSize())

	case keyMatches(msg, k.NextFile):
		m.selectFile(m.fileIndex + 1)
	case keyMatches(msg, k.PrevFile):
		m.selectFile(m.fileIndex - 1)

	case keyMatches(msg, k.Bottom):
		m.jumpTo(hadCount, count, true)
	case keyMatches(msg, k.Top): // wasPendingG, i.e. second "g" of "gg"
		m.jumpTo(hadCount, count, false)

	case keyMatches(msg, k.AddComment):
		if _, ok := m.currentLine(); ok {
			m.mode = modeComment
			m.input = ""
		}

	case keyMatches(msg, k.DeleteComment):
		m.deleteCommentUnderCursor()

	case keyMatches(msg, k.ToggleResolved):
		m.toggleResolvedUnderCursor()

	case keyMatches(msg, k.Refresh):
		m.refreshAll()

	case keyMatches(msg, k.Export):
		if path, err := exportSession(m.repoRoot, m.session, m.diffFiles(), formatMarkdown); err != nil {
			m.err = err
		} else {
			m.status = "exported to " + path
		}

	case keyMatches(msg, k.CopyMarkdown):
		if err := copyToClipboard(renderMarkdown(m.session, m.diffFiles())); err != nil {
			m.err = err
		} else {
			m.status = "copied review (markdown) to clipboard"
		}

	case keyMatches(msg, k.CopyJSON):
		data, err := renderJSON(m.session)
		if err == nil {
			err = copyToClipboard(string(data))
		}
		if err != nil {
			m.err = err
		} else {
			m.status = "copied review (json) to clipboard"
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

// jumpTo implements G/gg: with no count, jump to defaultLast (G's default)
// or the top (gg's default); with a count, both behave like vim's
// "{count}G" — jump to that 1-based row number, clamped to the file's
// extent.
func (m *model) jumpTo(hadCount bool, count int, defaultLast bool) {
	rows := m.currentRows()
	if len(rows) == 0 {
		return
	}
	if !hadCount {
		if defaultLast {
			m.lineIndex = len(rows) - 1
		} else {
			m.lineIndex = 0
		}
		return
	}
	idx := count - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	m.lineIndex = idx
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
// adopts whatever it persisted as m.session — so the in-memory copy can
// never drift from what's on disk, without a second, redundant read of the
// file we just wrote under lock.
func (m *model) mutateSession(fn func(Session) (Session, error)) {
	var saved Session
	err := withSession(m.repoRoot, func(s Session) (Session, error) {
		next, err := fn(s)
		if err != nil {
			return Session{}, err
		}
		saved = next
		return next, nil
	})
	if err != nil {
		m.err = err
		return
	}
	m.session = saved
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
