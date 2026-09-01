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

// runSkill is deliberately agent-agnostic: it just hands back the
// instructions, as a path or as raw text, and it's up to the user to point
// whatever agent they're using at it (paste the text in, tell the agent to
// read the path, etc) — it doesn't assume or install into any specific
// agent's own config/skill directory.
func runSkill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rv skill <path|print>")
	}
	switch args[0] {
	case "path":
		path := filepath.Join(cacheDir(), "rv-review-skill.md")
		// Always rewritten so it stays in sync with whatever rv binary is
		// currently installed, rather than going stale after an upgrade.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(skillMD), 0o644); err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	case "print":
		fmt.Print(skillMD)
		return nil
	default:
		return fmt.Errorf("unknown skill subcommand %q (want path|print)", args[0])
	}
}
