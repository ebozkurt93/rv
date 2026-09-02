package main

import "fmt"

// zshCompletion delegates rv's own completion to zsh's git completion,
// rather than reimplementing ref/branch listing: it rewrites the command
// line as if it were "git diff <what you typed>" and hands off to _git,
// which already knows how to complete branches, tags, remote refs, and
// range syntax (a..b / a...b) for a diff-like command. #compdef makes this
// file autoload for the "rv" command the moment it's found in $fpath — no
// eval/source line needed in .zshrc.
const zshCompletion = `#compdef rv

words=(git diff ${words[2,-1]})
(( CURRENT++ ))
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
