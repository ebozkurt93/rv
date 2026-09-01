package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// skillMD is embedded (rather than read from the source tree at runtime) so
// `rv skill path`/`rv skill print` work regardless of how rv was installed
// — nix, `make install`, or a bare `go build` — and regardless of whether
// the source repo is even still around.
//
//go:embed skills/rv-review/SKILL.md
var skillMD string

func runSkill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rv skill <path|print|install>")
	}
	switch args[0] {
	case "path":
		path := filepath.Join(cacheDir(), "rv-review-skill.md")
		// Always rewritten so it stays in sync with whatever rv binary is
		// currently installed, rather than going stale after an upgrade.
		if err := writeSkillFile(path); err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	case "print":
		fmt.Print(skillMD)
		return nil
	case "install":
		path := filepath.Join(os.Getenv("HOME"), ".claude", "skills", "rv-review", "SKILL.md")
		if err := writeSkillFile(path); err != nil {
			return err
		}
		fmt.Println("installed to", path)
		fmt.Println("Claude Code will pick it up as the \"rv-review\" skill in any project from now on.")
		return nil
	default:
		return fmt.Errorf("unknown skill subcommand %q (want path|print|install)", args[0])
	}
}

func writeSkillFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(skillMD), 0o644)
}
