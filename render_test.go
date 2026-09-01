package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
// renderComment/renderReply's own "  ▏..." indent convention, not a boxed
// dialog with its own border characters.
func TestCommentEditorHasNoBorder(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)
	m.width, m.height = 100, 24
	m.mode = modeComment
	m.input = "looks good"

	out := m.renderCommentEditor()
	for _, borderChar := range []string{panelBorder.TopLeft, panelBorder.Left} {
		if strings.Contains(out, borderChar) {
			t.Fatalf("expected no border characters in the comment editor, got %q", out)
		}
	}
	if !strings.Contains(out, "▏") {
		t.Fatalf("expected the same '▏' indent convention as renderComment/renderReply, got %q", out)
	}
	if !strings.Contains(out, "looks good") {
		t.Fatalf("expected the typed input to appear in the rendered editor, got %q", out)
	}
}
