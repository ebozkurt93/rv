package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// uiPrefs is the small set of display preferences that persist across
// sessions — not per-repo state (that's Session, in session.go), just "how
// the TUI looks," so a user doesn't have to re-toggle the sidebar or line
// numbers every time they open rv somewhere new.
type uiPrefs struct {
	SidebarHidden             bool `json:"sidebar_hidden"`
	ShowLineNumbers           bool `json:"show_line_numbers"`
	WrapLines                 bool `json:"wrap_lines"`
	ShowUntracked             bool `json:"show_untracked"`
	CommentNavIncludeResolved bool `json:"comment_nav_include_resolved"`
}

func defaultUIPrefs() uiPrefs {
	return uiPrefs{ShowLineNumbers: true}
}

func uiPrefsPath() string {
	return filepath.Join(configDir(), "ui-prefs.json")
}

// loadUIPrefs returns saved prefs, or the defaults if none have been saved
// yet or the file can't be read/parsed — a corrupt prefs file should never
// block startup.
func loadUIPrefs() uiPrefs {
	data, err := os.ReadFile(uiPrefsPath())
	if err != nil {
		return defaultUIPrefs()
	}
	var p uiPrefs
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultUIPrefs()
	}
	return p
}

func saveUIPrefs(p uiPrefs) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(uiPrefsPath(), data, 0o644)
}
