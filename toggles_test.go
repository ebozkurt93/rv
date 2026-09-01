package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestToggleSidebarHidesAndShowsPanel(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)
	if m.sidebarHidden {
		t.Fatal("expected sidebar visible by default")
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m2 := mm.(model)
	if !m2.sidebarHidden {
		t.Fatal("expected sidebar hidden after t")
	}

	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m3 := mm.(model)
	if m3.sidebarHidden {
		t.Fatal("expected sidebar visible again after second t")
	}
}

func TestTogglesPersistAcrossNewModel(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", nil, Session{}, nil)
	// Defaults: sidebar shown, line numbers on, wrap off. Toggle all three
	// away from their defaults.
	if m.sidebarHidden || !m.showLineNumbers || m.wrapLines {
		t.Fatalf("unexpected starting defaults: %+v", m.uiPrefs())
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m2 := mm.(model)
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("#")})
	m3 := mm.(model)
	mm, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m3b := mm.(model)
	mm, _ = m3b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	m4 := mm.(model)

	if !m4.sidebarHidden || m4.showLineNumbers || !m4.wrapLines || !m4.commentNavIncludeResolved {
		t.Fatalf("expected all four flipped from their defaults, got %+v", m4.uiPrefs())
	}

	// A fresh model (as if rv were relaunched) should pick up what was saved.
	reopened := newModel("/repo", nil, Session{}, nil)
	if !reopened.sidebarHidden || reopened.showLineNumbers || !reopened.wrapLines || !reopened.commentNavIncludeResolved {
		t.Fatalf("expected persisted prefs to carry over, got %+v", reopened.uiPrefs())
	}
}

func TestDefaultUIPrefsHasLineNumbersOnSidebarShown(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", nil, Session{}, nil)
	if m.sidebarHidden {
		t.Fatal("expected sidebar shown by default")
	}
	if !m.showLineNumbers {
		t.Fatal("expected line numbers shown by default")
	}
	if m.wrapLines {
		t.Fatal("expected wrap off by default")
	}
}

func TestDeleteCommentRequiresConfirmation(t *testing.T) {
	withTempHome(t)
	n := 1
	repo := "/repo"
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Body: "x"},
	}}))
	session, err := loadSession(repo)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, session, nil)
	m.lineIndex = 1 // first content line, which has NewLine=1

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m2 := mm.(model)
	if m2.mode != modeConfirmDelete {
		t.Fatalf("expected d to open confirm-delete prompt, got mode=%v", m2.mode)
	}
	if len(m2.session.Comments) != 1 {
		t.Fatal("expected comment NOT deleted yet, pending confirmation")
	}

	// 'n' cancels without deleting.
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m3 := mm.(model)
	if m3.mode != modeNormal || len(m3.session.Comments) != 1 {
		t.Fatalf("expected cancel to leave the comment alone, got mode=%v comments=%d", m3.mode, len(m3.session.Comments))
	}

	// 'd' then 'y' actually deletes.
	mm, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m4 := mm.(model)
	mm, _ = m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m5 := mm.(model)
	if m5.mode != modeNormal || len(m5.session.Comments) != 0 {
		t.Fatalf("expected 'y' to delete the comment, got mode=%v comments=%d", m5.mode, len(m5.session.Comments))
	}
}

func TestWrapLineSplitsAtWidth(t *testing.T) {
	got := wrapLine("aaaa bbbb cccc", 5)
	if len(got) < 2 {
		t.Fatalf("expected the line to wrap into multiple pieces, got %v", got)
	}
}

func TestRenderLineWithNumbersIncludesBothSides(t *testing.T) {
	old, new := 5, 7
	out := renderLine(pickLexer("a.txt"), Line{Kind: LineContext, Content: "x", OldLine: &old, NewLine: &new}, true, 1, false)
	if !containsAll(out, "5", "7", "x") {
		t.Fatalf("expected gutter to show both line numbers, got %q", out)
	}

	withoutNumbers := renderLine(pickLexer("a.txt"), Line{Kind: LineContext, Content: "x", OldLine: &old, NewLine: &new}, false, 1, false)
	if containsAll(withoutNumbers, "5", "7") {
		t.Fatalf("expected no line numbers when showNumbers=false, got %q", withoutNumbers)
	}
}

func TestGutterWidthScalesToFileSize(t *testing.T) {
	small := flattenFile(fileDiffWithLines("a.go", 8)) // max line number 8 -> 1 digit
	if small.numWidth != 1 {
		t.Fatalf("expected numWidth=1 for an 8-line file, got %d", small.numWidth)
	}

	big := flattenFile(fileDiffWithLines("a.go", 1234)) // max line number 1234 -> 4 digits
	if big.numWidth != 4 {
		t.Fatalf("expected numWidth=4 for a 1234-line file, got %d", big.numWidth)
	}

	old, new := 3, 3
	smallGutter := renderLine(pickLexer("a.txt"), Line{Kind: LineContext, Content: "x", OldLine: &old, NewLine: &new}, true, small.numWidth, false)
	bigGutter := renderLine(pickLexer("a.txt"), Line{Kind: LineContext, Content: "x", OldLine: &old, NewLine: &new}, true, big.numWidth, false)
	if len(smallGutter) >= len(bigGutter) {
		t.Fatalf("expected the small file's gutter to be narrower: small=%q big=%q", smallGutter, bigGutter)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
