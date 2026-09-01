package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestHelpOverlayTogglesWithoutChangingFrameHeight(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 5)}, Session{}, nil)
	m.width, m.height = 100, 24

	normalHeight := len(strings.Split(m.View(), "\n"))

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2, ok := mm.(model)
	if !ok {
		t.Fatalf("Update did not return a model")
	}
	if m2.mode != modeHelp {
		t.Fatalf("expected modeHelp after '?', got %v", m2.mode)
	}

	helpHeight := len(strings.Split(m2.View(), "\n"))
	if normalHeight != helpHeight {
		t.Fatalf("frame height changed opening help: normal=%d help=%d", normalHeight, helpHeight)
	}
}

func TestHelpOverlayClosesOnEscHelpOrQuestionMark(t *testing.T) {
	base := newModel("/repo", nil, Session{}, nil)
	base.mode = modeHelp

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyRunes, Runes: []rune("?")},
	} {
		mm, _ := base.updateHelp(key)
		m := mm.(model)
		if m.mode != modeNormal {
			t.Fatalf("expected %v to close help, mode is still %v", key, m.mode)
		}
	}
}

func TestHelpOverlayIgnoresUnrelatedKeys(t *testing.T) {
	base := newModel("/repo", nil, Session{}, nil)
	base.mode = modeHelp

	mm, _ := base.updateHelp(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m := mm.(model)
	if m.mode != modeHelp {
		t.Fatalf("expected an unrelated key to leave help open, got mode=%v", m.mode)
	}
}

// TestHelpOverlayScrollsPastOneScreen guards the fix for "? help overlay
// needs to scroll" — helpRows() has more entries than fit on a short
// terminal, and before this fix renderHelp truncated via fitBlock instead
// of scrolling, so entries past the visible height were unreachable.
func TestHelpOverlayScrollsPastOneScreen(t *testing.T) {
	m := newModel("/repo", nil, Session{}, nil)
	m.width, m.height = 80, 10 // short enough that helpLines() overflows one screen
	m.mode = modeHelp

	if len(m.helpLines()) <= m.bodyHeight()-borderOverheadH {
		t.Fatalf("test needs a terminal short enough that help overflows one screen")
	}

	mm, _ := m.updateHelp(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := mm.(model)
	if m2.helpScroll != 1 {
		t.Fatalf("expected 'j' to scroll the help overlay down by 1, got helpScroll=%d", m2.helpScroll)
	}

	// Scrolling can't run past the point where the last line is on screen.
	for i := 0; i < 1000; i++ {
		mm, _ = m2.updateHelp(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m2 = mm.(model)
	}
	maxScroll := len(m2.helpLines()) - (m2.bodyHeight() - borderOverheadH)
	if m2.helpScroll != maxScroll {
		t.Fatalf("expected scrolling to clamp at %d, got %d", maxScroll, m2.helpScroll)
	}

	mm, _ = m2.updateHelp(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m3 := mm.(model)
	if m3.helpScroll != maxScroll-1 {
		t.Fatalf("expected 'k' to scroll back up by 1, got helpScroll=%d", m3.helpScroll)
	}
}

// TestCommentEditorHasNoBorder guards the fix for "comment editor shouldn't
// be a bordered box" — it should read as an inline row matching
// renderComment/renderReply's own "  │..." indent convention, not a boxed
// dialog with its own border characters.
func TestCommentEditorHasNoBorder(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)
	m.width, m.height = 100, 24
	m.mode = modeComment
	m.input = "looks good"

	out := m.renderCommentEditor()
	// Corner characters are unambiguous evidence of an actual bordered box
	// — unlike panelBorder.Left, which happens to be the same "│" glyph the
	// comment thread's own connector column deliberately reuses (see
	// renderComment), so checking for that would false-positive now.
	if strings.Contains(out, panelBorder.TopLeft) {
		t.Fatalf("expected no border characters in the comment editor, got %q", out)
	}
	if !strings.Contains(out, "│") {
		t.Fatalf("expected the same '│' indent convention as renderComment/renderReply, got %q", out)
	}
	if !strings.Contains(out, "looks good") {
		t.Fatalf("expected the typed input to appear in the rendered editor, got %q", out)
	}
}

// commentConnectorColumn returns the cell column of the first "│" in s, or
// -1 if there isn't one — used to check the connector stays in the same
// column across a comment's continuation lines and its replies, tree-style,
// rather than comparing byte offsets (the prefix has multi-byte runes, so a
// byte offset would overcount relative to actual terminal columns).
func commentConnectorColumn(s string) int {
	idx := strings.IndexAny(s, "│├└")
	if idx < 0 {
		return -1
	}
	return ansi.StringWidth(s[:idx])
}

// TestCommentContinuationLineKeepsConnectorColumn guards the fix for "so
// comment indicator is still not aligned... I want something similar to
// what tree command has" — a multi-line comment's continuation rows (and
// its replies) must keep "│" in the same column as the first line's, like
// `tree` keeps "│" in a fixed column down consecutive rows, instead of the
// connector appearing to end after the first line.
func TestCommentContinuationLineKeepsConnectorColumn(t *testing.T) {
	// Reply bodies here are deliberately single-line — a reply's own
	// wrapped continuation doesn't carry the connector forward (there's
	// nothing branching further off a leaf), so only the comment's own
	// continuation and its replies' first lines are expected to align.
	c := Comment{Author: "user", Body: "first line\nsecond line",
		Replies: []Reply{{Author: "agent", Body: "a reply"}, {Author: "user", Body: "another reply"}}}

	rows := strings.Split(renderComment(c), "\n")
	for i, r := range c.Replies {
		last := i == len(c.Replies)-1
		rows = append(rows, strings.Split(renderReply(r, false, last), "\n")...)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 rendered rows, got %d: %v", len(rows), rows)
	}

	want := commentConnectorColumn(rows[0])
	if want < 0 {
		t.Fatalf("expected the first row to contain a '│' connector: %q", rows[0])
	}
	for _, row := range rows[1:] {
		if got := commentConnectorColumn(row); got != want {
			t.Fatalf("expected '│' at column %d on every row, got %d in %q", want, got, row)
		}
	}
}

// TestReplyBranchesLikeTree guards the fix for the "└" appearing on every
// reply regardless of position — only the last reply should cap the thread
// with "└─"; every earlier one gets "├─" so the connector keeps running to
// whatever follows, exactly like `tree` branches a directory listing.
func TestReplyBranchesLikeTree(t *testing.T) {
	first := renderReply(Reply{Author: "agent", Body: "a"}, false, false)
	last := renderReply(Reply{Author: "user", Body: "b"}, false, true)
	if !strings.Contains(first, "├─") {
		t.Fatalf("expected a non-last reply to branch with '├─', got %q", first)
	}
	if !strings.Contains(last, "└─") {
		t.Fatalf("expected the last reply to cap the thread with '└─', got %q", last)
	}
}

// TestComposingReplyKeepsConnectorRunning guards the fix for a visible
// "gap": composing a new reply renders below the existing ones, so the
// previously-last reply must flip from "└─" to "├─" (something now follows
// it) and the in-progress editor row must itself become the new "└"
// terminal branch — otherwise the thread looks like it already ended right
// before more of it appears below. Exercises the real buildDiffLinesDetailed
// path, not a re-implementation of its branch logic.
func TestComposingReplyKeepsConnectorRunning(t *testing.T) {
	n := 1
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Author: "user", Body: "original",
			Replies: []Reply{{ID: "r_1", Author: "agent", Body: "first reply"}}},
	}}, nil)
	m.width = 100
	m.lineIndex = 1 // the commented line
	m.mode = modeComment
	m.replyingToCommentID = "c_1"
	m.input = "another reply"

	lines, _, _, _ := m.buildDiffLinesDetailed(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "├─") {
		t.Fatalf("expected the previously-last reply to branch with '├─' while a new one is being composed, got %q", joined)
	}
	if !strings.Contains(joined, "└─ ↩") {
		t.Fatalf("expected the in-progress reply row to use the terminal branch '└', got %q", joined)
	}
}

func TestReplyStyleReflectsThreadResolution(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	reply := Reply{Author: "agent", Body: "on it"}

	unresolved := renderReply(reply, false, true)
	resolved := renderReply(reply, true, true)
	if unresolved == resolved {
		t.Fatalf("expected resolved vs unresolved threads to style a reply differently")
	}
}
