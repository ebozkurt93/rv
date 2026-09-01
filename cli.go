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

const usage = `rv — local git diff review, with agent handoff

You review a diff in a terminal UI and leave comments anchored to lines; a
coding agent reads and replies to those comments from the same shell via the
subcommands below, without any server or shared account — everything lives
in one JSON session file per repo checkout, under ~/.cache/rv/<hash>/,
guarded by a file lock so the TUI and the CLI can be used at the same time.

Usage:
  rv [diff-spec...]     Launch the TUI. diff-spec is passed straight through
                         to 'git diff' — e.g. HEAD~3, main..feature, --staged,
                         a..b — instead of rv reimplementing revision syntax.
                         With none given, defaults to the working tree vs
                         HEAD (uncommitted changes, staged and unstaged).

Agent-facing subcommands (human-readable text by default; add --json for
machine-readable output, meant for scripting):
  rv session get [--json]
                         Session metadata: repo root, comment counts.
  rv comment list [--file F] [--unresolved] [--json]
                         List comments (optionally filtered) — each has an
                         id, file, line, body, author, and resolved flag,
                         plus any threaded replies.
  rv comment resolve <id>
                         Mark a comment resolved.
  rv comment reply <id> <text...> [--author NAME] [--resolve]
                         Append a threaded reply to a comment — author
                         defaults to "agent" if not given — shows up in the
                         TUI within ~1s. --resolve also marks it resolved.
  rv skill print         Print the full agent workflow guide — the
                         canonical, complete instructions for how an agent
                         should discover, read, and act on rv comments in a
                         repo. Read this instead of guessing at the flow.
  rv skill path          Print the path to that same guide on disk.

Other:
  rv version             Print the version.
  rv -h, --help          Show this help.

Inside the TUI, press ? for the full keybinding reference; comments and
replies made in the TUI show up to 'rv comment list' immediately, and vice
versa (the TUI polls the session file roughly once a second).`

func run(args []string) error {
	if len(args) == 0 {
		return runTUI(nil)
	}

	switch {
	case args[0] == "version" || args[0] == "--version" || args[0] == "-v":
		fmt.Println("rv", version)
		return nil
	case args[0] == "help" || args[0] == "--help" || args[0] == "-h":
		fmt.Println(usage)
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

	keymap, keymapErr := loadKeymap()

	m := newModel(repoRoot, diffFiles, session, normalizedSpec)
	m.keys = keymap
	if keymapErr != nil {
		// Bad config shouldn't block startup — fall back to defaults (which
		// loadKeymap already returned) and just surface the error so it's
		// visible instead of silently ignored.
		m.err = keymapErr
	}

	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
