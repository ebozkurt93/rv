package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExportNoOpsWhenNoComments(t *testing.T) {
	withTempHome(t)
	repo := "/repo"
	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, Session{RepoRoot: repo}, nil)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m2 := mm.(model)

	if m2.err != nil {
		t.Fatalf("expected no error, got %v", m2.err)
	}
	if m2.status == "" || m2.status == "exported to "+filepath.Join(sessionDir(repo), "review.md") {
		t.Fatalf("expected a 'nothing to export' status, got %q", m2.status)
	}
	if _, err := os.Stat(filepath.Join(sessionDir(repo), "review.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no review.md to be written, stat error: %v", err)
	}
}

func TestCopyMarkdownNoOpsWhenNoComments(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m2 := mm.(model)

	if m2.status != "nothing to copy — no comments yet" {
		t.Fatalf("expected the nothing-to-copy status, got %q", m2.status)
	}
}

func TestCopyJSONNoOpsWhenNoComments(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m2 := mm.(model)

	if m2.status != "nothing to copy — no comments yet" {
		t.Fatalf("expected the nothing-to-copy status, got %q", m2.status)
	}
}

func TestExportStillWorksWithComments(t *testing.T) {
	withTempHome(t)
	repo := "/repo"
	n := 1
	session := Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Body: "hi"},
	}}
	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, session, nil)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m2 := mm.(model)

	if m2.err != nil {
		t.Fatalf("expected no error, got %v", m2.err)
	}
	if _, err := os.Stat(filepath.Join(sessionDir(repo), "review.md")); err != nil {
		t.Fatalf("expected review.md to be written, got error: %v", err)
	}
}
