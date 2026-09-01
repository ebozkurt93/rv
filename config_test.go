package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseKeymapConfigBasic(t *testing.T) {
	src := `
# a comment line
quit = q, ctrl+c
move_down = w   # trailing comment

move_up = e
`
	got, err := parseKeymapConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got["quit"]) != 2 || got["quit"][0] != "q" || got["quit"][1] != "ctrl+c" {
		t.Fatalf("unexpected quit binding: %v", got["quit"])
	}
	if len(got["move_down"]) != 1 || got["move_down"][0] != "w" {
		t.Fatalf("unexpected move_down binding: %v", got["move_down"])
	}
	if len(got["move_up"]) != 1 || got["move_up"][0] != "e" {
		t.Fatalf("unexpected move_up binding: %v", got["move_up"])
	}
}

func TestParseKeymapConfigRejectsMalformedLine(t *testing.T) {
	if _, err := parseKeymapConfig("not a valid line"); err == nil {
		t.Fatal("expected an error for a line with no '='")
	}
}

func TestParseKeymapConfigRejectsEmptyKeyList(t *testing.T) {
	if _, err := parseKeymapConfig("quit = \n"); err == nil {
		t.Fatal("expected an error for an action with no keys")
	}
}

func TestLoadKeymapMissingFileReturnsDefaults(t *testing.T) {
	withTempHome(t)
	km, err := loadKeymap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if km.MoveDown[0] != "j" {
		t.Fatalf("expected default keymap, got MoveDown=%v", km.MoveDown)
	}
}

func TestLoadKeymapAppliesOverrides(t *testing.T) {
	withTempHome(t)
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "move_down = w\nmove_up = e\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	km, err := loadKeymap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(km.MoveDown) != 1 || km.MoveDown[0] != "w" {
		t.Fatalf("expected MoveDown overridden to [w], got %v", km.MoveDown)
	}
	if len(km.MoveUp) != 1 || km.MoveUp[0] != "e" {
		t.Fatalf("expected MoveUp overridden to [e], got %v", km.MoveUp)
	}
	// Untouched actions keep their defaults.
	if km.Quit[0] != "q" {
		t.Fatalf("expected Quit to keep its default, got %v", km.Quit)
	}
}

func TestLoadKeymapUnknownActionErrorsButStillReturnsWorkingDefaults(t *testing.T) {
	withTempHome(t)
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("not_a_real_action = x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	km, err := loadKeymap()
	if err == nil {
		t.Fatal("expected an error for an unknown action name")
	}
	if km.MoveDown[0] != "j" {
		t.Fatalf("expected a still-usable default keymap despite the error, got %v", km.MoveDown)
	}
}

func TestHelpRowsReflectRemappedKeys(t *testing.T) {
	m := newModel("/repo", nil, Session{}, nil)
	m.keys.MoveDown = []string{"w"}

	rows := m.helpRows()
	found := false
	for _, r := range rows {
		if len(r.keys) == 1 && r.keys[0] == "w" && r.desc != "" {
			found = true
		}
		if len(r.keys) == 1 && r.keys[0] == "j" {
			t.Fatalf("expected the stale default 'j' to be gone from help rows, found in %+v", r)
		}
	}
	if !found {
		t.Fatal("expected the remapped 'w' key to appear in help rows")
	}
}
