package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println("rv", version)
		return nil
	case "session":
		return runSession(args[1:])
	case "comment":
		return runComment(args[1:])
	case "skill":
		return runSkill(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// loadDiffFiles re-runs and re-parses `git diff HEAD` against the current
// working tree — used both for the TUI's initial load and for its manual
// refresh (see (*model).refreshAll in update.go).
func loadDiffFiles() ([]FileDiff, error) {
	raw, err := (gitVCS{runner: gitRunner}).Diff()
	if err != nil {
		return nil, err
	}
	return ParseDiff(raw)
}

func runTUI() error {
	repoRoot, err := currentRepoRoot()
	if err != nil {
		return err
	}

	diffFiles, err := loadDiffFiles()
	if err != nil {
		return fmt.Errorf("parsing diff: %w", err)
	}

	session, err := loadSession(repoRoot)
	if err != nil {
		return err
	}

	m := newModel(repoRoot, diffFiles, session)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
