package main

import "fmt"

// zshCompletion delegates rv's own completion to zsh's git completion,
// rather than reimplementing ref/branch listing: it rewrites the command
// line as if it were "git diff <what you typed>" and hands off to _git,
// which already knows how to complete branches, tags, remote refs, and
// range syntax (a..b / a...b) for a diff-like command. #compdef makes this
// file autoload for the "rv" command the moment it's found in $fpath — no
// eval/source line needed in .zshrc.
//
// service=git is load-bearing, not decoration: zsh's own _git dispatcher
// only handles top-level "git <cmd>" parsing when $service == git;
// otherwise (its default here, since zsh sets $service to the compdef'd
// command — "rv") it falls through to `_call_function ret _$service`,
// which is just _rv again — infinite mutual recursion between _rv and
// _git ("maximum nested function level reached"), reproduced and
// confirmed against the actual _git source before this fix.
//
// Everything is also `local` so a plugin that re-triggers completion on
// every keystroke (autosuggest-style, not just on Tab) can't have one
// invocation's mutated words/CURRENT/service leak into the next one's —
// each call starts from the real, unmodified outer context.
const zshCompletion = `#compdef rv

local service=git
local -a args
args=(${words[2,-1]})
local -a words=(git diff $args)
local CURRENT=$(( CURRENT + 1 ))
_git
`

// bashCompletion is zshCompletion's bash equivalent: __git_complete is
// bash-completion's own contrib helper, built exactly for wrapping a
// non-git command that takes git-like revision arguments. Requires
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
