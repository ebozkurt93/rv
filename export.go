package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type exportFormat int

const (
	formatMarkdown exportFormat = iota
	formatJSON
)

// exportSession renders session as format and writes it into the session's
// cache dir (review.md or review.json), returning the path written. files is
// the current diff, used to quote each comment's line in the markdown
// output.
func exportSession(repoRoot string, session Session, files []FileDiff, format exportFormat) (string, error) {
	dir := sessionDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	var name string
	var data []byte
	var err error

	switch format {
	case formatJSON:
		name = "review.json"
		data, err = renderJSON(session)
	default:
		name = "review.md"
		data = []byte(renderMarkdown(session, files))
	}
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func renderJSON(session Session) ([]byte, error) {
	return json.MarshalIndent(session.Comments, "", "  ")
}

// renderMarkdown produces a handoff doc grouped by file, each comment
// anchored with file:line and quoting the line it's attached to — meant to
// be pasted into (or read directly by) a coding agent.
func renderMarkdown(session Session, files []FileDiff) string {
	byFile := map[string][]Comment{}
	var order []string
	for _, c := range session.Comments {
		if _, ok := byFile[c.File]; !ok {
			order = append(order, c.File)
		}
		byFile[c.File] = append(byFile[c.File], c)
	}

	lineText := indexLineContent(files)

	var b strings.Builder
	total, unresolved := len(session.Comments), 0
	for _, c := range session.Comments {
		if !c.Resolved {
			unresolved++
		}
	}
	b.WriteString("# Code review\n\n")
	fmt.Fprintf(&b, "%d comment(s), %d unresolved.\n\n", total, unresolved)

	if total == 0 {
		b.WriteString("No comments.\n")
		return b.String()
	}

	for _, file := range order {
		fmt.Fprintf(&b, "## %s\n\n", file)
		for _, c := range byFile[file] {
			line := commentLineNumber(c)
			quoted := lineText[fileLineKey{file, line}]
			status := ""
			if c.Resolved {
				status = " (resolved)"
			}
			fmt.Fprintf(&b, "- **%s:%d**%s (%s): %s\n", file, line, status, c.Author, c.Body)
			if quoted != "" {
				fmt.Fprintf(&b, "  ```\n  %s\n  ```\n", quoted)
			}
			for _, r := range c.Replies {
				fmt.Fprintf(&b, "  - _%s_: %s\n", r.Author, r.Body)
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func commentLineNumber(c Comment) int {
	if c.NewLine != nil {
		return *c.NewLine
	}
	if c.OldLine != nil {
		return *c.OldLine
	}
	return 0
}

type fileLineKey struct {
	file string
	line int
}

// indexLineContent maps (file, line-number-as-a-comment-would-record-it) to
// that line's text, so renderMarkdown can quote it without re-walking hunks
// per comment.
func indexLineContent(files []FileDiff) map[fileLineKey]string {
	out := map[fileLineKey]string{}
	for _, f := range files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l.NewLine != nil {
					out[fileLineKey{f.Path, *l.NewLine}] = l.Content
				}
				if l.OldLine != nil {
					if _, exists := out[fileLineKey{f.Path, *l.OldLine}]; !exists {
						out[fileLineKey{f.Path, *l.OldLine}] = l.Content
					}
				}
			}
		}
	}
	return out
}
