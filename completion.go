package main

import "fmt"

// zshCompletion delegates rv's own completion to zsh's git completion,
// rather than reimplementing ref/branch listing: it rewrites the command
// line as if it were "git diff <what you typed>" and hands off to _git,
// which already knows how to complete branches, tags, remote refs, and
// range syntax (a..b / a...b) for a diff-like command.
//
// Meant for `eval "$(rv completion zsh)"` in .zshrc — same idiom as
// `eval "$(starship init zsh)"`/`eval "$(atuin init zsh)"` — rather than a
// #compdef file dropped somewhere on $fpath: that would tie completion to
// wherever it happened to be installed (a package manager's site-functions
// dir) and go stale silently if this script ever changes, since nothing
// would re-generate it. eval always re-runs against whatever `rv` is
// currently on $PATH, and works the same regardless of install method
// (Homebrew, Nix, `go install`, a manual build).
//
// compdef (a runtime call here, not a #compdef file header) only exists
// once compinit has run — unlike starship/atuin's own eval lines, which
// don't depend on the completion system at all, so there's no shared
// convention to lean on for "make sure this line comes after your
// compinit call." There's no way to make this genuinely free of that
// ordering requirement (compdef isn't defined until compinit's own
// autoloaded file has actually executed at least once — checked directly
// against zsh's own compinit source; there's no built-in "queue calls
// made before I've run" mechanism to lean on instead), so rather than
// push the ordering requirement onto whoever's .zshrc this ends up in,
// the script checks for compdef itself and runs compinit if it's missing.
// That call uses -C (trust the existing completion cache, skipping the
// $fpath rescan/staleness check that makes a plain compinit call
// noticeably slower) to keep the common "placed before an existing,
// not-yet-run compinit call" case cheap — still a genuine correctness
// no-op once compinit has actually run elsewhere (compdef already exists,
// this whole line short-circuits).
//
// service=git is load-bearing, not decoration: zsh's own _git dispatcher
// only handles top-level "git <cmd>" parsing when $service == git;
// otherwise (its default here, since zsh sets $service to the compdef'd
// command — "rv") it falls through to `_call_function ret _$service`,
// which is just this function again — infinite mutual recursion with
// _git ("maximum nested function level reached"), reproduced and
// confirmed against the actual _git source before this fix.
//
// Everything inside the function is also `local` so a plugin that
// re-triggers completion on every keystroke (autosuggest-style, not just
// on Tab) can't have one invocation's mutated words/CURRENT/service leak
// into the next one's — each call starts from the real, unmodified outer
// context.
const zshCompletion = `(( $+functions[compdef] )) || { autoload -Uz compinit; compinit -C }
_rv_completion() {
  local service=git
  local -a args
  args=(${words[2,-1]})
  local -a words=(git diff $args)
  local CURRENT=$(( CURRENT + 1 ))
  _git
}
compdef _rv_completion rv
`

// bashCompletion is zshCompletion's bash equivalent: __git_complete is
// bash-completion's own contrib helper, built exactly for wrapping a
// non-git command that takes git-like revision arguments — already a
// direct runtime registration call, so `eval "$(rv completion bash)"` in
// .bashrc works as-is, no separate wrapper needed. Requires
// bash-completion's git support to already be loaded (true wherever git's
// own tab completion already works).
const bashCompletion = `# Requires bash-completion's git support to already be loaded.
__git_complete rv _git_diff
`

func runCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rv completion <bash|zsh>")
	}
	switch args[0] {
	case "zsh":
		fmt.Print(zshCompletion)
	case "bash":
		fmt.Print(bashCompletion)
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, zsh)", args[0])
	}
	return nil
}
