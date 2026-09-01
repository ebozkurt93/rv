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
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runTUI() error {
	vcs := gitVCS{runner: gitRunner}

	repoRoot, err := vcs.RepoRoot()
	if err != nil {
		return fmt.Errorf("not a git repo: %w", err)
	}

	raw, err := vcs.Diff()
	if err != nil {
		return err
	}
	diffFiles, err := ParseDiff(raw)
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
