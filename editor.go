package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editorClosedMsg reports the outcome of a suspend-into-$EDITOR round trip
// (see openInEditorCmd) once control returns to the TUI.
type editorClosedMsg struct{ err error }

// resolveEditor is the $VISUAL/$EDITOR/vi fallback chain git itself uses.
func resolveEditor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// editorCommand resolves which editor to launch and how to tell it to open
// at a specific line. Most terminal editors (vim, nvim, nano) accept "+N
// file" to land on line N; VS Code needs its own "-g file:line" form
// instead.
func editorCommand(path string, line int) *exec.Cmd {
	editor := resolveEditor()
	if base := filepath.Base(editor); base == "code" || base == "code-insiders" {
		return exec.Command(editor, "-g", fmt.Sprintf("%s:%d", path, line))
	}
	return exec.Command(editor, fmt.Sprintf("+%d", line), path)
}

// editorCommandForComment is editorCommand without a line (a fresh comment
// body has nothing to jump to), but critically adds --wait for VS Code —
// unlike a terminal editor, `code` returns almost immediately without it,
// which would read the temp file back before the user had actually edited
// it (see openCommentInEditorCmd).
func editorCommandForComment(path string) *exec.Cmd {
	editor := resolveEditor()
	if base := filepath.Base(editor); base == "code" || base == "code-insiders" {
		return exec.Command(editor, "--wait", path)
	}
	return exec.Command(editor, path)
}

// openInEditorCmd suspends the TUI (tea.ExecProcess correctly restores raw
// terminal mode afterward — required for a full-screen editor like vim to
// take over the terminal cleanly) and hands off to editorCommand.
func openInEditorCmd(path string, line int) tea.Cmd {
	cmd := editorCommand(path, line)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorClosedMsg{err: err}
	})
}

// commentEditorClosedMsg reports the outcome of editing an in-progress
// comment body in $EDITOR (see openCommentInEditorCmd) — content is the
// temp file's contents once the editor closes, to load back into the
// comment being composed.
type commentEditorClosedMsg struct {
	content string
	err     error
}

// openCommentInEditorCmd writes current to a temp file, suspends the TUI,
// and opens it in $EDITOR — same suspend/restore mechanism as
// openInEditorCmd, but reads the result back afterward instead of just
// reporting success, so a comment can be composed with the user's real
// editor (multi-line, syntax highlighting, whatever they're used to) rather
// than the single-line inline input.
func openCommentInEditorCmd(current string) tea.Cmd {
	f, err := os.CreateTemp("", "rv-comment-*.md")
	if err != nil {
		return func() tea.Msg { return commentEditorClosedMsg{err: err} }
	}
	path := f.Name()
	_, writeErr := f.WriteString(current)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		if writeErr == nil {
			writeErr = closeErr
		}
		return func() tea.Msg { return commentEditorClosedMsg{err: writeErr} }
	}

	cmd := editorCommandForComment(path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return commentEditorClosedMsg{err: err}
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return commentEditorClosedMsg{err: readErr}
		}
		return commentEditorClosedMsg{content: strings.TrimRight(string(data), "\n")}
	})
}

// editorLineFor picks which line number to hand the editor: the new-side
// line if the cursor is on one (added or context), otherwise the old-side
// line (a removed line has no current-file position, so this is only an
// approximation), otherwise line 1.
func editorLineFor(l Line) int {
	if l.NewLine != nil {
		return *l.NewLine
	}
	if l.OldLine != nil {
		return *l.OldLine
	}
	return 1
}
