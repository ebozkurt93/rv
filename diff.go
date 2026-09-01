package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/waigani/diffparser"
)

// VCS abstracts "get me a unified diff of the working copy against its
// parent" so the parsing/TUI code doesn't care which tool produced it. git is
// the only implementation today; jj can join later (e.g. via `jj diff --git`,
// which already emits git-compatible unified diffs) without touching
// ParseDiff or anything downstream of it.
type VCS interface {
	RepoRoot() (string, error)
	Diff() (string, error)
}

// GitRunner runs a git subcommand and returns its stdout.
type GitRunner interface {
	Run(args ...string) (string, error)
}

type execGitRunner struct{}

func (execGitRunner) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

var gitRunner GitRunner = execGitRunner{}

// gitVCS is the git-backed VCS implementation. Diff scope for v1 is the
// working tree vs HEAD (staged + unstaged changes to tracked files).
type gitVCS struct {
	runner GitRunner
}

func (g gitVCS) RepoRoot() (string, error) {
	out, err := g.runner.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g gitVCS) Diff() (string, error) {
	return g.runner.Run("diff", "HEAD", "--no-color", "--no-ext-diff")
}

// LineKind identifies which side(s) of a diff a line belongs to.
type LineKind int

const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
)

// Line is a single line within a hunk, with old/new line numbers filled in
// for whichever side(s) it applies to (context lines have both).
type Line struct {
	Kind    LineKind
	Content string
	OldLine *int
	NewLine *int
}

// Hunk is a contiguous block of changed (and surrounding context) lines.
type Hunk struct {
	Header string
	Lines  []Line
}

// FileStatus describes how a file changed.
type FileStatus int

const (
	FileModified FileStatus = iota
	FileAdded
	FileDeleted
)

// FileDiff is all the hunks for one file.
type FileDiff struct {
	Path    string // new path (or old path, for deleted files)
	OldPath string
	Status  FileStatus
	Hunks   []Hunk
}

// ParseDiff parses a unified diff (as produced by `git diff` or `jj diff
// --git`) into our domain types.
func ParseDiff(raw string) ([]FileDiff, error) {
	parsed, err := diffparser.Parse(raw)
	if err != nil {
		return nil, err
	}

	files := make([]FileDiff, 0, len(parsed.Files))
	for _, f := range parsed.Files {
		fd := FileDiff{
			Path:    f.NewName,
			OldPath: f.OrigName,
			Status:  fileStatus(f.Mode),
		}
		if fd.Path == "" {
			fd.Path = fd.OldPath
		}

		for _, h := range f.Hunks {
			fd.Hunks = append(fd.Hunks, convertHunk(h))
		}
		files = append(files, fd)
	}
	return files, nil
}

func fileStatus(m diffparser.FileMode) FileStatus {
	switch m {
	case diffparser.NEW:
		return FileAdded
	case diffparser.DELETED:
		return FileDeleted
	default:
		return FileModified
	}
}

// convertHunk walks a hunk's lines in order, tracking old/new line counters
// itself rather than trusting diffparser's per-line Number (which, for
// unchanged lines, only ever records the new-side number).
func convertHunk(h *diffparser.DiffHunk) Hunk {
	hunk := Hunk{Header: h.HunkHeader}
	oldLine := h.OrigRange.Start
	newLine := h.NewRange.Start

	for _, dl := range h.WholeRange.Lines {
		var line Line
		line.Content = dl.Content

		switch dl.Mode {
		case diffparser.ADDED:
			line.Kind = LineAdded
			n := newLine
			line.NewLine = &n
			newLine++
		case diffparser.REMOVED:
			line.Kind = LineRemoved
			o := oldLine
			line.OldLine = &o
			oldLine++
		default: // UNCHANGED
			line.Kind = LineContext
			o, n := oldLine, newLine
			line.OldLine = &o
			line.NewLine = &n
			oldLine++
			newLine++
		}

		hunk.Lines = append(hunk.Lines, line)
	}

	return hunk
}
