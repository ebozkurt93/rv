package main

import (
	"os"
	"testing"
)

func withEditorEnv(t *testing.T, visual, editor string) {
	t.Helper()
	prevV, prevE := os.Getenv("VISUAL"), os.Getenv("EDITOR")
	os.Setenv("VISUAL", visual)
	os.Setenv("EDITOR", editor)
	t.Cleanup(func() {
		os.Setenv("VISUAL", prevV)
		os.Setenv("EDITOR", prevE)
	})
}

func TestEditorCommandUsesPlusLineForTerminalEditors(t *testing.T) {
	withEditorEnv(t, "", "nvim")
	cmd := editorCommand("/repo/foo.go", 42)
	if len(cmd.Args) != 3 || cmd.Args[1] != "+42" || cmd.Args[2] != "/repo/foo.go" {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}
}

func TestEditorCommandUsesDashGForVSCode(t *testing.T) {
	withEditorEnv(t, "", "code")
	cmd := editorCommand("/repo/foo.go", 42)
	if len(cmd.Args) != 3 || cmd.Args[1] != "-g" || cmd.Args[2] != "/repo/foo.go:42" {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}
}

func TestEditorCommandVisualTakesPrecedenceOverEditor(t *testing.T) {
	withEditorEnv(t, "nvim", "nano")
	cmd := editorCommand("/repo/foo.go", 1)
	if cmd.Path == "" {
		t.Fatal("expected a resolved command path")
	}
	if got := cmd.Args[0]; got != "nvim" {
		t.Fatalf("expected VISUAL (nvim) to win over EDITOR (nano), got %q", got)
	}
}

func TestEditorCommandFallsBackToVi(t *testing.T) {
	withEditorEnv(t, "", "")
	cmd := editorCommand("/repo/foo.go", 1)
	if cmd.Args[0] != "vi" {
		t.Fatalf("expected fallback to vi, got %q", cmd.Args[0])
	}
}

func TestEditorLineForPrefersNewLine(t *testing.T) {
	old, new := 5, 7
	if got := editorLineFor(Line{OldLine: &old, NewLine: &new}); got != 7 {
		t.Fatalf("expected NewLine (7) preferred, got %d", got)
	}
	if got := editorLineFor(Line{OldLine: &old}); got != 5 {
		t.Fatalf("expected OldLine fallback (5), got %d", got)
	}
	if got := editorLineFor(Line{}); got != 1 {
		t.Fatalf("expected default of 1, got %d", got)
	}
}
