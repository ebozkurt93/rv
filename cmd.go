package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// currentRepoRoot resolves the repo root the same way the TUI does, so a CLI
// invocation run from within the same working directory always addresses
// the same session.
func currentRepoRoot() (string, error) {
	root, err := (gitVCS{runner: gitRunner}).RepoRoot()
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}
	return root, nil
}

func runSession(args []string) error {
	if len(args) == 0 || args[0] != "get" {
		return fmt.Errorf("usage: rv session get [--json]")
	}
	fs := flag.NewFlagSet("session get", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	repoRoot, err := currentRepoRoot()
	if err != nil {
		return err
	}
	session, err := loadSession(repoRoot)
	if err != nil {
		return err
	}

	total, unresolved := len(session.Comments), 0
	for _, c := range session.Comments {
		if !c.Resolved {
			unresolved++
		}
	}

	if *jsonOut {
		return printJSON(struct {
			RepoRoot   string `json:"repo_root"`
			Comments   int    `json:"comments"`
			Unresolved int    `json:"unresolved"`
		}{session.RepoRoot, total, unresolved})
	}
	fmt.Printf("repo:       %s\n", session.RepoRoot)
	fmt.Printf("comments:   %d\n", total)
	fmt.Printf("unresolved: %d\n", unresolved)
	return nil
}

func runComment(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rv comment <list|resolve|reply> ...")
	}
	switch args[0] {
	case "list":
		return runCommentList(args[1:])
	case "resolve":
		return runCommentResolve(args[1:])
	case "reply":
		return runCommentReply(args[1:])
	default:
		return fmt.Errorf("unknown comment subcommand %q", args[0])
	}
}

func runCommentList(args []string) error {
	fs := flag.NewFlagSet("comment list", flag.ContinueOnError)
	file := fs.String("file", "", "only comments on this file")
	unresolvedOnly := fs.Bool("unresolved", false, "only unresolved comments")
	jsonOut := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoRoot, err := currentRepoRoot()
	if err != nil {
		return err
	}
	session, err := loadSession(repoRoot)
	if err != nil {
		return err
	}

	comments := filterComments(session.Comments, *file, *unresolvedOnly)

	if *jsonOut {
		return printJSON(comments)
	}
	printCommentsText(comments)
	return nil
}

func filterComments(comments []Comment, file string, unresolvedOnly bool) []Comment {
	out := make([]Comment, 0, len(comments))
	for _, c := range comments {
		if file != "" && c.File != file {
			continue
		}
		if unresolvedOnly && c.Resolved {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return commentLineNumber(out[i]) < commentLineNumber(out[j])
	})
	return out
}

func printCommentsText(comments []Comment) {
	if len(comments) == 0 {
		fmt.Println("no comments")
		return
	}
	for _, c := range comments {
		status := "unresolved"
		if c.Resolved {
			status = "resolved"
		}
		fmt.Printf("%s [%s] %s:%d\n", c.ID, status, c.File, commentLineNumber(c))
		fmt.Printf("  %s: %s\n", c.Author, c.Body)
		for _, r := range c.Replies {
			fmt.Printf("  └─ %s: %s\n", r.Author, r.Body)
		}
	}
}

func runCommentResolve(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rv comment resolve <id>")
	}
	id := args[0]

	repoRoot, err := currentRepoRoot()
	if err != nil {
		return err
	}
	return withSession(repoRoot, func(s Session) (Session, error) {
		for i := range s.Comments {
			if s.Comments[i].ID == id {
				s.Comments[i].Resolved = true
				return s, nil
			}
		}
		return Session{}, fmt.Errorf("no comment with id %q", id)
	})
}

// parseReplyArgs pulls --author/--resolve out of args wherever they appear,
// rather than using flag.FlagSet (which stops recognizing flags at the
// first positional argument — and here the first positional argument, the
// comment id, necessarily comes before the reply text, so an agent placing
// --resolve at the end, as in `reply <id> <text> --resolve`, would
// otherwise have it silently swallowed into the reply body instead of
// applied).
func parseReplyArgs(args []string) (author string, resolve bool, rest []string, err error) {
	author = "agent"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--resolve":
			resolve = true
		case a == "--author":
			if i+1 >= len(args) {
				return "", false, nil, fmt.Errorf("--author requires a value")
			}
			i++
			author = args[i]
		case strings.HasPrefix(a, "--author="):
			author = strings.TrimPrefix(a, "--author=")
		default:
			rest = append(rest, a)
		}
	}
	return author, resolve, rest, nil
}

func runCommentReply(args []string) error {
	author, resolve, rest, err := parseReplyArgs(args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return fmt.Errorf("usage: rv comment reply <id> <text...> [--author=agent] [--resolve]")
	}
	id := rest[0]
	body := strings.Join(rest[1:], " ")

	repoRoot, err := currentRepoRoot()
	if err != nil {
		return err
	}
	return withSession(repoRoot, func(s Session) (Session, error) {
		for i := range s.Comments {
			if s.Comments[i].ID == id {
				s.Comments[i].Replies = append(s.Comments[i].Replies, Reply{
					ID:        newReplyID(),
					Body:      body,
					Author:    author,
					CreatedAt: time.Now(),
				})
				if resolve {
					s.Comments[i].Resolved = true
				}
				return s, nil
			}
		}
		return Session{}, fmt.Errorf("no comment with id %q", id)
	})
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
