package main

import (
	"strings"
	"testing"
)

// TestRunCompletionZshDelegatesToGit guards the actual mechanism: rv's own
// completion must not reimplement ref/branch listing — it rewrites the
// command line as "git diff <args>" and hands off to zsh's own _git, which
// already knows how to complete branches, tags, remote refs, and a..b/
// a...b ranges.
func TestRunCompletionZshDelegatesToGit(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runCompletion([]string{"zsh"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.HasPrefix(out, "#compdef rv") {
		t.Fatalf("expected a #compdef header so this autoloads for the rv command, got %q", out)
	}
	if !strings.Contains(out, "git diff") || !strings.Contains(out, "_git") {
		t.Fatalf("expected delegation to git's own diff completion, got %q", out)
	}
}

// TestRunCompletionZshSetsServiceToAvoidInfiniteRecursion guards a real,
// reproduced bug: zsh's own _git dispatcher only handles "git <cmd>"
// parsing when $service == git; since zsh sets $service to the compdef'd
// command name ("rv") by default, calling _git without first setting
// service=git makes _git fall through to `_call_function ret _$service` —
// which is just _rv again, causing infinite mutual recursion between _rv
// and _git ("maximum nested function level reached").
func TestRunCompletionZshSetsServiceToAvoidInfiniteRecursion(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runCompletion([]string{"zsh"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "service=git") {
		t.Fatalf("expected service to be explicitly set to git before delegating, got %q", out)
	}
	if !strings.Contains(out, "local") {
		t.Fatalf("expected words/CURRENT/service to be scoped local so repeated invocations (e.g. an autosuggest-style plugin firing on every keystroke) can't leak mutated state into each other, got %q", out)
	}
}

func TestRunCompletionBashDelegatesToGit(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runCompletion([]string{"bash"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "__git_complete rv _git_diff") {
		t.Fatalf("expected the bash-completion __git_complete delegation, got %q", out)
	}
}

func TestRunCompletionRejectsUnknownShell(t *testing.T) {
	if err := runCompletion([]string{"fish"}); err == nil {
		t.Fatalf("expected an error for an unsupported shell")
	}
}

func TestRunCompletionRequiresExactlyOneArg(t *testing.T) {
	if err := runCompletion(nil); err == nil {
		t.Fatalf("expected a usage error with no shell given")
	}
	if err := runCompletion([]string{"zsh", "extra"}); err == nil {
		t.Fatalf("expected a usage error with extra args")
	}
}

// TestCompletionDispatchesAsSubcommand guards the CLI wiring itself: "rv
// completion zsh" must reach runCompletion via dispatchSubcommand, not
// fall through to being treated as a diff-spec.
func TestCompletionDispatchesAsSubcommand(t *testing.T) {
	out := captureStdout(t, func() {
		if err := run([]string{"completion", "zsh"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.HasPrefix(out, "#compdef rv") {
		t.Fatalf("expected the zsh completion script, got %q", out)
	}
}
