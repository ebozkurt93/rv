package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configDir follows the XDG convention (unlike cacheDir, which mirrors
// tmux-mover's plain ~/.cache/<name> — config and cache are different
// concerns with different expectations: config is meant to be
// hand-edited and versioned, cache is disposable).
func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "rv")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "rv")
}

func configPath() string {
	return filepath.Join(configDir(), "config")
}

// loadKeymap returns defaultKeymap() with any overrides from the config
// file applied on top. Returns the (possibly-default) keymap plus an error
// if the file exists but is malformed — callers should still get a working
// keymap back either way and surface the error rather than fail to start.
func loadKeymap() (Keymap, error) {
	km := defaultKeymap()

	data, err := os.ReadFile(configPath())
	if os.IsNotExist(err) {
		return km, nil
	}
	if err != nil {
		return km, err
	}

	overrides, err := parseKeymapConfig(string(data))
	if err != nil {
		return km, err
	}

	fields := keymapFields(&km)
	for action, keys := range overrides {
		ptr, ok := fields[action]
		if !ok {
			return km, fmt.Errorf("config: unknown action %q (see ? in the TUI for valid actions/keys)", action)
		}
		*ptr = keys
	}
	return km, nil
}

// parseKeymapConfig parses the simple "action = key1, key2" format, one
// binding per line. "#" starts a comment (whole-line or trailing); blank
// lines are ignored. No sections, no quoting — this is meant to be a
// two-minute edit, not a format to learn.
func parseKeymapConfig(s string) (map[string][]string, error) {
	out := map[string][]string{}
	for i, line := range strings.Split(s, "\n") {
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("config line %d: expected \"action = keys\", got %q", i+1, line)
		}
		action := strings.TrimSpace(parts[0])

		var keys []string
		for _, k := range strings.Split(parts[1], ",") {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("config line %d: %q has no keys", i+1, action)
		}
		out[action] = keys
	}
	return out, nil
}

// keymapFields maps each config action name to the Keymap field it
// controls, so loadKeymap can apply overrides without reflection.
func keymapFields(km *Keymap) map[string]*[]string {
	return map[string]*[]string{
		"quit":                 &km.Quit,
		"move_down":            &km.MoveDown,
		"move_up":              &km.MoveUp,
		"half_page_down":       &km.HalfPageDown,
		"half_page_up":         &km.HalfPageUp,
		"next_file":            &km.NextFile,
		"prev_file":            &km.PrevFile,
		"next_hunk":            &km.NextHunk,
		"prev_hunk":            &km.PrevHunk,
		"next_comment":         &km.NextComment,
		"prev_comment":         &km.PrevComment,
		"top":                  &km.Top,
		"bottom":               &km.Bottom,
		"add_comment":          &km.AddComment,
		"delete_comment":       &km.DeleteComment,
		"toggle_resolved":      &km.ToggleResolved,
		"toggle_reviewed":      &km.ToggleReviewed,
		"open_editor":          &km.OpenEditor,
		"refresh":              &km.Refresh,
		"export":               &km.Export,
		"copy_markdown":        &km.CopyMarkdown,
		"copy_json":            &km.CopyJSON,
		"help":                 &km.Help,
		"search":               &km.Search,
		"toggle_sidebar":       &km.ToggleSidebar,
		"toggle_line_numbers":  &km.ToggleLineNumbers,
		"toggle_wrap":          &km.ToggleWrap,
		"confirm":              &km.Confirm,
		"cancel":               &km.Cancel,
		"backspace":            &km.Backspace,
		"comment_newline":      &km.CommentNewline,
		"comment_editor":       &km.CommentEditor,
		"toggle_comment_scope": &km.ToggleCommentScope,
		"toggle_untracked":     &km.ToggleUntracked,
		"clear_session":        &km.ClearSession,
		"toggle_split_view":    &km.ToggleSplitView,
	}
}
