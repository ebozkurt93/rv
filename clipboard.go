package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard copies text to the system clipboard by shelling out to
// whatever clipboard utility the platform provides — there's no portable
// stdlib way to do this, and pulling in a CGo clipboard binding would
// complicate the nix build for little benefit over what's already on PATH.
func copyToClipboard(text string) error {
	cmd, err := clipboardCmd()
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%s: %s", cmd.Path, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func clipboardCmd() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("pbcopy"), nil
	case "linux":
		for _, c := range [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		} {
			if path, err := exec.LookPath(c[0]); err == nil {
				return exec.Command(path, c[1:]...), nil
			}
		}
		return nil, fmt.Errorf("no clipboard utility found (install wl-copy, xclip, or xsel)")
	case "windows":
		return exec.Command("clip"), nil
	default:
		return nil, fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
}
