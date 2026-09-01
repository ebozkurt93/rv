package main

import (
	"fmt"
	"path/filepath"
	"sort"
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
	case editorClosedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.status = "back from editor"
		}
		return m, nil
	case commentEditorClosedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else if m.mode == modeComment {
			m.input = msg.content
		}
		return m, nil
	case tea.MouseMsg:
		switch m.mode {
		case modeNormal:
			m.handleMouse(msg)
		case modeHelp:
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.scrollHelp(-3)
			case tea.MouseButtonWheelDown:
				m.scrollHelp(3)
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeComment:
			return m.updateComment(msg)
		case modeHelp:
			return m.updateHelp(msg)
		case modeSearch:
			return m.updateSearch(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		case modeConfirmClearSession:
			return m.updateConfirmClearSession(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

// updateHelp handles input while the help overlay is open: the keys that
// could plausibly mean "close this" do so, movement keys scroll the list
// (it can run past one screen — see the "?" help overlay TODO item), and
// anything else is ignored so a stray keypress while skimming can't
// accidentally trigger some other action.
func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case keyMatches(msg, k.Help), keyMatches(msg, k.Cancel), keyMatches(msg, k.Quit):
		m.mode = modeNormal
	case keyMatches(msg, k.MoveDown):
		m.scrollHelp(1)
	case keyMatches(msg, k.MoveUp):
		m.scrollHelp(-1)
	case keyMatches(msg, k.HalfPageDown):
		m.scrollHelp(m.halfPageSize())
	case keyMatches(msg, k.HalfPageUp):
		m.scrollHelp(-m.halfPageSize())
	}
	return m, nil
}

// scrollHelp adjusts m.helpScroll by delta (negative scrolls up), clamped
// so it can't run past either end of helpLines() — shared by updateHelp's
// keyboard handling and the mouse wheel (see handleMouse) so both paths
// can't disagree about where the clamp is.
func (m *model) scrollHelp(delta int) {
	m.helpScroll += delta
	if max := len(m.helpLines()) - (m.bodyHeight() - borderOverheadH); m.helpScroll > max {
		m.helpScroll = max
	}
	if m.helpScroll < 0 {
		m.helpScroll = 0
	}
}

// updateSearch handles the sidebar file-name filter prompt (/). The sidebar
// already shows live-filtered results as m.input changes (see
// renderSidebar); Confirm commits m.input into the persisted m.fileFilter
// and snaps the cursor onto a now-visible file, Cancel leaves m.fileFilter
// exactly as it was before "/" was pressed.
func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case keyMatches(msg, k.Cancel):
		m.mode = modeNormal
		m.input = ""
	case keyMatches(msg, k.Confirm):
		m.fileFilter = m.input
		m.snapToVisibleFile()
		m.mode = modeNormal
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

// updateConfirmDelete handles the "delete this comment? y/n" prompt shown
// after d — deleting a comment is destructive and a single keystroke, so it
// gets a confirmation rather than firing immediately.
func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.deleteCommentUnderCursor()
		m.mode = modeNormal
	case "n", "N", "esc":
		m.mode = modeNormal
	}
	return m, nil
}

// updateConfirmClearSession handles the "clear all comments and reviewed
// marks for this session? y/n" prompt — wiping every comment in the repo's
// session is destructive and easy to trigger by accident, so like deleting
// a single comment it gets a confirmation rather than firing immediately.
func (m model) updateConfirmClearSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.clearSession()
		m.mode = modeNormal
	case "n", "N", "esc":
		m.mode = modeNormal
	}
	return m, nil
}

// clearSession wipes every comment and reviewed-file mark for the current
// repo's session — useful for anyone repeatedly reviewing diffs against the
// same working directory, where leftover comments from a previous review
// would otherwise keep piling up (see keys.ClearSession).
func (m *model) clearSession() {
	m.mutateSession(func(s Session) (Session, error) {
		s.Comments = nil
		s.Reviewed = nil
		return s, nil
	})
	m.status = "session cleared"
}

// persistUIPrefs saves the model's current display prefs (sidebar
// visibility, line numbers, wrap) so the next `rv` invocation — anywhere,
// not just this repo — starts with them already applied.
func (m *model) persistUIPrefs() {
	if err := saveUIPrefs(m.uiPrefs()); err != nil {
		m.err = err
	}
}

// refreshAll re-reads both the diff (the working tree may have changed —
// e.g. an agent editing files while this TUI sits open — which the
// background session poll never covers, since it only watches
// comments/replies) and the session, without restarting the program.
func (m *model) refreshAll() {
	diffFiles, err := loadDiffFiles(m.diffSpec)
	if err != nil {
		m.err = err
		return
	}
	m.trackedDiffs = diffFiles
	// Best-effort: an untracked-file scan failing shouldn't block refreshing
	// the tracked diff, so just keep whatever untrackedDiffs already had.
	if untracked, uerr := untrackedFileDiffs(gitRunner, m.repoRoot); uerr == nil {
		m.untrackedDiffs = untracked
	}
	m.setDiffFiles(m.effectiveDiffFiles())
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
		m.selectVisibleFile(1)
	case keyMatches(msg, k.PrevFile):
		m.selectVisibleFile(-1)

	case keyMatches(msg, k.NextHunk):
		for i := 0; i < count; i++ {
			m.jumpToHunk(1)
		}
	case keyMatches(msg, k.PrevHunk):
		for i := 0; i < count; i++ {
			m.jumpToHunk(-1)
		}

	case keyMatches(msg, k.NextComment):
		m.jumpToComment(1)
	case keyMatches(msg, k.PrevComment):
		m.jumpToComment(-1)
	case keyMatches(msg, k.ToggleCommentScope):
		m.commentNavIncludeResolved = !m.commentNavIncludeResolved
		m.persistUIPrefs()
		if m.commentNavIncludeResolved {
			m.status = "n/N now include resolved comments"
		} else {
			m.status = "n/N: unresolved comments only"
		}

	case keyMatches(msg, k.Bottom):
		m.jumpTo(hadCount, count, true)
	case keyMatches(msg, k.Top): // wasPendingG, i.e. second "g" of "gg"
		m.jumpTo(hadCount, count, false)

	case keyMatches(msg, k.AddComment):
		if _, ok := m.currentLine(); ok {
			m.mode = modeComment
			m.input = ""
			m.editingCommentID = ""
			m.editingReplyID = ""
			m.replyingToCommentID = ""
			c, editing, editingReplyID, replying := m.commentActionForCurrentLine()
			switch {
			case editing:
				m.input = c.Body
				m.editingCommentID = c.ID
			case editingReplyID != "":
				m.editingReplyID = editingReplyID
				for _, r := range c.Replies {
					if r.ID == editingReplyID {
						m.input = r.Body
						break
					}
				}
			case replying:
				m.replyingToCommentID = c.ID
			}
		}

	case keyMatches(msg, k.DeleteComment):
		if len(m.commentsForCurrentLine()) > 0 {
			m.mode = modeConfirmDelete
		}

	case keyMatches(msg, k.ClearSession):
		if len(m.session.Comments) > 0 || len(m.session.Reviewed) > 0 {
			m.mode = modeConfirmClearSession
		}

	case keyMatches(msg, k.ToggleResolved):
		m.toggleResolvedUnderCursor()

	case keyMatches(msg, k.ToggleReviewed):
		m.toggleCurrentFileReviewed()

	case keyMatches(msg, k.OpenEditor):
		if line, ok := m.currentLine(); ok {
			path := filepath.Join(m.repoRoot, m.files[m.fileIndex].file.Path)
			return m, openInEditorCmd(path, editorLineFor(line))
		}

	case keyMatches(msg, k.Refresh):
		m.refreshAll()

	case keyMatches(msg, k.Help):
		m.mode = modeHelp
		m.helpScroll = 0

	case keyMatches(msg, k.Search):
		m.mode = modeSearch
		m.input = m.fileFilter // editing starts from whatever filter is already active

	case keyMatches(msg, k.ToggleSidebar):
		m.sidebarHidden = !m.sidebarHidden
		m.persistUIPrefs()

	case keyMatches(msg, k.ToggleLineNumbers):
		m.showLineNumbers = !m.showLineNumbers
		m.persistUIPrefs()

	case keyMatches(msg, k.ToggleWrap):
		m.wrapLines = !m.wrapLines
		m.persistUIPrefs()

	case keyMatches(msg, k.ToggleUntracked):
		m.toggleShowUntracked()
		m.persistUIPrefs()
		if m.showUntracked {
			m.status = fmt.Sprintf("showing %d untracked file(s)", len(m.untrackedDiffs))
		} else {
			m.status = "hiding untracked files"
		}

	case keyMatches(msg, k.Export):
		if len(m.session.Comments) == 0 {
			m.status = "nothing to export — no comments yet"
			break
		}
		if path, err := exportSession(m.repoRoot, m.session, m.diffFiles(), formatMarkdown); err != nil {
			m.err = err
		} else {
			m.status = "exported to " + path
		}

	case keyMatches(msg, k.CopyMarkdown):
		if len(m.session.Comments) == 0 {
			m.status = "nothing to copy — no comments yet"
			break
		}
		if err := copyToClipboard(renderMarkdown(m.session, m.diffFiles())); err != nil {
			m.err = err
		} else {
			m.status = "copied review (markdown) to clipboard"
		}

	case keyMatches(msg, k.CopyJSON):
		if len(m.session.Comments) == 0 {
			m.status = "nothing to copy — no comments yet"
			break
		}
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
		m.editingCommentID = ""
		m.editingReplyID = ""
		m.replyingToCommentID = ""
	case keyMatches(msg, k.Confirm):
		m.addCommentUnderCursor()
		m.mode = modeNormal
		m.input = ""
		m.editingCommentID = ""
		m.editingReplyID = ""
		m.replyingToCommentID = ""
	case keyMatches(msg, k.CommentNewline):
		m.input += "\n"
	case keyMatches(msg, k.CommentEditor):
		return m, openCommentInEditorCmd(m.input)
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
			m.lineIndex = lastContentRow(rows)
		} else {
			m.lineIndex = firstContentRow(rows)
		}
		return
	}
	m.lineIndex = nearestContentRow(rows, count-1)
}

// jumpToHunk moves the cursor to the next (dir>0) or previous (dir<0) hunk
// within the current file, landing on its first line (a header can't be
// commented on, so stopping there just costs an extra j/k every time). Runs
// off the end of the current file's hunks into the next/prev visible file
// instead of stopping — see jumpToHunkAcrossFiles.
func (m *model) jumpToHunk(dir int) {
	rows := m.currentRows()
	i := m.lineIndex
	for {
		i += dir
		if i < 0 || i >= len(rows) {
			m.jumpToHunkAcrossFiles(dir)
			return
		}
		if rows[i].kind != rowHunkHeader {
			continue
		}
		target := i
		if next := i + 1; next < len(rows) && rows[next].kind == rowLine {
			target = next
		}
		// If that first line is where we already are (we're searching
		// backward and just hit our own hunk's header, since we're normally
		// sitting right after it), keep looking past it — otherwise "{"
		// would look like a no-op instead of moving to the previous hunk.
		if target == m.lineIndex {
			continue
		}
		m.lineIndex = target
		return
	}
}

// jumpToHunkAcrossFiles is jumpToHunk's fallback once it runs off the
// current file's hunks: move to the first hunk of the next visible file
// (dir>0) or the last hunk of the previous one (dir<0), wrapping around at
// either end like tab/[/] and n/N already do. A no-op if there's no other
// visible file to go to.
func (m *model) jumpToHunkAcrossFiles(dir int) {
	vis := m.visibleFileIndices(m.fileFilter)
	if len(vis) <= 1 {
		return
	}
	pos := 0
	for i, v := range vis {
		if v == m.fileIndex {
			pos = i
			break
		}
	}
	pos = ((pos+dir)%len(vis) + len(vis)) % len(vis)
	newFileIdx := vis[pos]

	rows := m.files[newFileIdx].rows
	var target int
	if dir > 0 {
		target = firstContentRow(rows)
	} else {
		target = lastHunkFirstContentRow(rows)
	}
	m.fileIndex = newFileIdx
	m.lineIndex = target
}

// lastHunkFirstContentRow finds the last hunk header in rows and returns
// the index of its first content line — the backward counterpart of
// firstContentRow, used when crossing into the previous file so "{" lands
// consistently on a hunk's start rather than the file's last line.
func lastHunkFirstContentRow(rows []diffRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].kind == rowHunkHeader {
			if next := i + 1; next < len(rows) && rows[next].kind == rowLine {
				return next
			}
			break
		}
	}
	return lastContentRow(rows)
}

// jumpToComment moves the cursor to the next (dir>0) or previous (dir<0)
// unresolved comment, across all files in sidebar order, wrapping around at
// either end — resolved comments are skipped since there's nothing left to
// act on there.
func (m *model) jumpToComment(dir int) {
	type loc struct{ fileIdx, rowIdx int }
	var locs []loc
	for _, c := range m.session.Comments {
		if c.Resolved && !m.commentNavIncludeResolved {
			continue
		}
		if fi, ri, ok := m.locateComment(c); ok {
			locs = append(locs, loc{fi, ri})
		}
	}
	if len(locs) == 0 {
		return
	}
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].fileIdx != locs[j].fileIdx {
			return locs[i].fileIdx < locs[j].fileIdx
		}
		return locs[i].rowIdx < locs[j].rowIdx
	})

	before := func(a, b loc) bool {
		if a.fileIdx != b.fileIdx {
			return a.fileIdx < b.fileIdx
		}
		return a.rowIdx < b.rowIdx
	}
	cur := loc{m.fileIndex, m.lineIndex}

	var target loc
	if dir > 0 {
		target = locs[0] // wrap
		for _, l := range locs {
			if before(cur, l) {
				target = l
				break
			}
		}
	} else {
		target = locs[len(locs)-1] // wrap
		for i := len(locs) - 1; i >= 0; i-- {
			if before(locs[i], cur) {
				target = locs[i]
				break
			}
		}
	}
	m.fileIndex, m.lineIndex = target.fileIdx, target.rowIdx
}

// moveCursor moves delta commentable rows up/down, transparently stepping
// over hunk-header rows along the way — the cursor lands only on content,
// never rests on a header, whether it's passing through one on its way
// somewhere else or would otherwise land exactly on one.
func (m *model) moveCursor(delta int) {
	rows := m.currentRows()
	if len(rows) == 0 {
		return
	}
	dir, n := 1, delta
	if delta < 0 {
		dir, n = -1, -delta
	}
	cur := m.lineIndex
	for i := 0; i < n; i++ {
		next := cur
		for {
			next += dir
			if next < 0 || next >= len(rows) {
				next = cur // boundary — nothing further that direction
				break
			}
			if rows[next].kind == rowLine {
				break
			}
		}
		if next == cur {
			break
		}
		cur = next
	}
	m.lineIndex = cur
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
	m.lineIndex = firstContentRow(m.files[idx].rows)
}

// selectVisibleFile moves the selection by step among files that pass the
// current fileFilter, rather than every file — so tab/[/] skip over
// filtered-out entries instead of landing on one that isn't even shown.
// Wraps at either end (tab past the last file goes to the first, and vice
// versa) rather than clamping, matching how n/N already wrap for comments.
func (m *model) selectVisibleFile(step int) {
	vis := m.visibleFileIndices(m.fileFilter)
	if len(vis) == 0 {
		return
	}
	pos := 0
	for i, v := range vis {
		if v == m.fileIndex {
			pos = i
			break
		}
	}
	pos = ((pos+step)%len(vis) + len(vis)) % len(vis)
	m.selectFile(vis[pos])
}

// snapToVisibleFile moves the selection onto the first file that passes the
// current fileFilter, if the currently-selected one doesn't (e.g. right
// after confirming a new filter, or after a refresh changes what's there).
func (m *model) snapToVisibleFile() {
	vis := m.visibleFileIndices(m.fileFilter)
	if len(vis) == 0 {
		return
	}
	for _, v := range vis {
		if v == m.fileIndex {
			return
		}
	}
	m.selectFile(vis[0])
}

// commentActionForCurrentLine decides what pressing AddComment on the
// cursor's line should do, for the user's own most recent comment there:
//
//   - Nothing's replied to it yet: edit its body in place (editing=true).
//   - The thread's last message is itself a reply the user wrote, with
//     nothing after it: edit THAT reply's body in place instead
//     (editingReplyID set) — otherwise the only way to fix a typo in your
//     own latest reply would be to add yet another one on top of it.
//   - Someone else (an agent) spoke last: add a new reply (replying=true).
//     Rewriting the body of a message someone already responded to would
//     silently change what they were responding to — a new message reads
//     as "and separately, ..." instead.
//   - Nothing of the user's is on this line yet: all three false, start a
//     brand new comment.
//
// A resolved match is treated the same as an unresolved one: editing/
// replying un-resolves it (see addCommentUnderCursor) rather than requiring
// a separate "reopen" step or starting a second, parallel thread.
func (m model) commentActionForCurrentLine() (comment Comment, editing bool, editingReplyID string, replying bool) {
	comments := m.commentsForCurrentLine()
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if c.Author != "user" {
			continue
		}
		if len(c.Replies) == 0 {
			return c, true, "", false
		}
		if last := c.Replies[len(c.Replies)-1]; last.Author == "user" {
			return c, false, last.ID, false
		}
		return c, false, "", true
	}
	return Comment{}, false, "", false
}

func (m *model) addCommentUnderCursor() {
	if m.input == "" {
		return
	}

	switch {
	case m.editingCommentID != "":
		id, body := m.editingCommentID, m.input
		m.mutateSession(func(s Session) (Session, error) {
			for i := range s.Comments {
				if s.Comments[i].ID == id {
					s.Comments[i].Body = body
					s.Comments[i].Resolved = false
					break
				}
			}
			return s, nil
		})

	case m.editingReplyID != "":
		id, body := m.editingReplyID, m.input
		m.mutateSession(func(s Session) (Session, error) {
			for i := range s.Comments {
				for j := range s.Comments[i].Replies {
					if s.Comments[i].Replies[j].ID == id {
						s.Comments[i].Replies[j].Body = body
						s.Comments[i].Resolved = false
						return s, nil
					}
				}
			}
			return s, nil
		})

	case m.replyingToCommentID != "":
		id, body := m.replyingToCommentID, m.input
		reply := Reply{ID: newReplyID(), Body: body, Author: "user", CreatedAt: time.Now()}
		m.mutateSession(func(s Session) (Session, error) {
			for i := range s.Comments {
				if s.Comments[i].ID == id {
					s.Comments[i].Replies = append(s.Comments[i].Replies, reply)
					s.Comments[i].Resolved = false
					break
				}
			}
			return s, nil
		})

	default:
		line, ok := m.currentLine()
		if !ok {
			return
		}
		file := m.files[m.fileIndex].file.Path

		comment := Comment{
			ID:          newCommentID(),
			File:        file,
			OldLine:     line.OldLine,
			NewLine:     line.NewLine,
			LineContent: line.Content,
			Body:        m.input,
			Author:      "user",
			CreatedAt:   time.Now(),
		}

		m.mutateSession(func(s Session) (Session, error) {
			s.Comments = append(s.Comments, comment)
			return s, nil
		})
	}
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

// toggleCurrentFileReviewed marks the selected file reviewed (recording its
// current content hash) or, if it's already reviewed, un-marks it. Marking
// reviewed is what needs the fresh hash — if the file changes afterward,
// isFileReviewed already treats a hash mismatch as unreviewed on its own,
// so there's nothing to actively invalidate later.
func (m *model) toggleCurrentFileReviewed() {
	if m.fileIndex < 0 || m.fileIndex >= len(m.files) {
		return
	}
	fd := m.files[m.fileIndex].file
	reviewed := isFileReviewed(m.session, fd)

	m.mutateSession(func(s Session) (Session, error) {
		if reviewed {
			delete(s.Reviewed, fd.Path)
		} else {
			if s.Reviewed == nil {
				s.Reviewed = map[string]string{}
			}
			s.Reviewed[fd.Path] = fileDiffHash(fd)
		}
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
