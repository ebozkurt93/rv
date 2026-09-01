package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// knownSubcommands are rv's own subcommands. Anything else in args[0] is
// treated as a `git diff` spec (a ref, a "a..b" range, "--staged", etc.) and
// passed straight through — see runTUI.
var knownSubcommands = map[string]bool{
	"session": true,
	"comment": true,
	"skill":   true,
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI(nil)
	}

	switch {
	case args[0] == "version" || args[0] == "--version" || args[0] == "-v":
		fmt.Println("rv", version)
		return nil
	case knownSubcommands[args[0]]:
		return dispatchSubcommand(args[0], args[1:])
	default:
		return runTUI(args)
	}
}

func dispatchSubcommand(name string, rest []string) error {
	switch name {
	case "session":
		return runSession(rest)
	case "comment":
		return runComment(rest)
	case "skill":
		return runSkill(rest)
	default:
		return fmt.Errorf("unknown command %q", name)
	}
}

// loadDiffFiles re-runs and re-parses `git diff <diffSpec>` against the
// current working tree — used both for the TUI's initial load and for its
// manual refresh (see (*model).refreshAll in update.go). A nil/empty
// diffSpec means the default: working tree vs HEAD.
func loadDiffFiles(diffSpec []string) ([]FileDiff, error) {
	raw, err := (gitVCS{runner: gitRunner, spec: diffSpec}).Diff()
	if err != nil {
		return nil, err
	}
	return ParseDiff(raw)
}

func runTUI(diffSpec []string) error {
	repoRoot, err := currentRepoRoot()
	if err != nil {
		return err
	}

	// Normalized once here so the header label (which reads m.diffSpec
	// directly) shows exactly what's actually being run, rather than the
	// raw user input gitVCS.DiffSpec() would otherwise silently correct
	// only at the moment it shells out.
	normalizedSpec := (gitVCS{spec: diffSpec}).DiffSpec()

	diffFiles, err := loadDiffFiles(normalizedSpec)
	if err != nil {
		return fmt.Errorf("parsing diff: %w", err)
	}

	session, err := loadSession(repoRoot)
	if err != nil {
		return err
	}

	m := newModel(repoRoot, diffFiles, session, normalizedSpec)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
