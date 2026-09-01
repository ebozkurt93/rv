package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// editorClosedMsg reports the outcome of a suspend-into-$EDITOR round trip
// (see openInEditorCmd) once control returns to the TUI.
type editorClosedMsg struct{ err error }

// editorCommand resolves which editor to launch and how to tell it to open
// at a specific line, following the same $VISUAL/$EDITOR/vi fallback chain
// git itself uses. Most terminal editors (vim, nvim, nano) accept "+N file"
// to land on line N; VS Code needs its own "-g file:line" form instead.
func editorCommand(path string, line int) *exec.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	if base := filepath.Base(editor); base == "code" || base == "code-insiders" {
		return exec.Command(editor, "-g", fmt.Sprintf("%s:%d", path, line))
	}
	return exec.Command(editor, fmt.Sprintf("+%d", line), path)
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
