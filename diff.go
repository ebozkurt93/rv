package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

// gitVCS is the git-backed VCS implementation. Spec is passed straight
// through as extra arguments to `git diff` — e.g. []string{"HEAD"} (the
// default), []string{"--staged"}, []string{"main..feature"}, or
// []string{"HEAD~3"} — rather than rv parsing and reimplementing any of
// git's own revision syntax.
type gitVCS struct {
	runner GitRunner
	spec   []string
}

func (g gitVCS) RepoRoot() (string, error) {
	out, err := g.runner.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DiffSpec returns the effective spec, defaulting to "HEAD" (working tree
// vs HEAD) when none was given, and normalizing any case variant of the
// word "head" within it to canonical "HEAD".
//
// This isn't cosmetic: on a case-insensitive filesystem (macOS's default),
// git worktrees have their own reproducible footgun here. rev-parse's first
// lookup strategy for a bare revision name x is the literal path
// $GIT_DIR/x; a worktree's $GIT_DIR is the per-worktree
// .git/worktrees/<name>/ directory, and on a case-insensitive filesystem
// $GIT_DIR/head resolves to the same inode $GIT_DIR/HEAD *would* if it
// existed there — except a worktree's real HEAD file lives one directory
// up in the shared repo, so this lookup instead falls through to some
// other candidate ref/object entirely. Confirmed against a real worktree:
// `git rev-parse HEAD` and `git rev-parse head` returned two different
// commits — "head" silently resolved to the wrong ref rather than erroring,
// which is far worse than the "no diff" it presents as.
func (g gitVCS) DiffSpec() []string {
	spec := g.spec
	if len(spec) == 0 {
		spec = []string{"HEAD"}
	}
	out := make([]string, len(spec))
	for i, s := range spec {
		out[i] = normalizeHeadCasing(s)
	}
	return out
}

// rangeSepPattern splits a diff spec on git's range separators. Order
// matters: alternation in Go's regexp is leftmost-first, so "\.\.\." must
// come before "\.\." or a "..." range would only ever consume 2 of its 3
// dots.
var rangeSepPattern = regexp.MustCompile(`\.\.\.|\.\.`)

// normalizeHeadCasing rewrites "head" to "HEAD" in s, but only where "head"
// is truly standing in for the special ref — the whole base name of s or of
// one side of a "a..b"/"a...b" range, optionally followed by a revision
// suffix operator (~N, ^N, @{...}). A word-boundary regex isn't precise
// enough here: "\bhead\b" also matches inside "feature-head" (a perfectly
// legitimate, distinct branch name) since "-" counts as a boundary same as
// "." does. Left alone entirely: flags (anything starting with "-"), and
// any name where "head" is merely a prefix of something else, like
// "headless" — there rest is "less", not empty or a suffix operator, so it
// doesn't qualify.
func normalizeHeadCasing(s string) string {
	if strings.HasPrefix(s, "-") {
		return s
	}
	parts := splitKeepSep(s, rangeSepPattern)
	for i, p := range parts {
		parts[i] = normalizeHeadBaseName(p)
	}
	return strings.Join(parts, "")
}

// splitKeepSep splits s on sep's matches, keeping each separator as its own
// element interleaved between the parts it split — e.g. for the ".."/"..."
// pattern, "head..main" -> ["head", "..", "main"].
func splitKeepSep(s string, sep *regexp.Regexp) []string {
	idx := sep.FindAllStringIndex(s, -1)
	if len(idx) == 0 {
		return []string{s}
	}
	var parts []string
	last := 0
	for _, m := range idx {
		parts = append(parts, s[last:m[0]], s[m[0]:m[1]])
		last = m[1]
	}
	return append(parts, s[last:])
}

func normalizeHeadBaseName(p string) string {
	if len(p) < 4 || !strings.EqualFold(p[:4], "head") {
		return p
	}
	rest := p[4:]
	if rest == "" || strings.HasPrefix(rest, "~") || strings.HasPrefix(rest, "^") || strings.HasPrefix(rest, "@") {
		return "HEAD" + rest
	}
	return p
}

func (g gitVCS) Diff() (string, error) {
	args := append([]string{"diff"}, g.DiffSpec()...)
	args = append(args, "--no-color", "--no-ext-diff")
	return g.runner.Run(args...)
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
	// Highlight marks which runes of Content are part of an intraline
	// (word-level) diff against this line's paired line on the other side
	// of a modification — nil unless this line was paired by
	// applyIntralineHighlights, in which case it's the same rune-length as
	// Content.
	Highlight []bool
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
	// Binary is true for an untracked binary file (see untrackedFileDiff) —
	// its content isn't dumped as "every line added" the way a text file's
	// is; Hunks is empty and the diff pane shows a placeholder instead.
	Binary bool
	// Untracked is true for a file git isn't tracking at all (see
	// untrackedFileDiffs) — distinguished from Status == FileAdded, which
	// git diff itself reports for a file that's staged/tracked as new.
	Untracked bool
}

// fileDiffHash fingerprints a file's diff content (every hunk header and
// line — kind, content, and line numbers), so "mark reviewed" (see
// (*model).toggleFileReviewed) can tell a genuinely unchanged file from one
// that only looks the same at a glance. Any change at all — including
// something that shifts line numbers without touching this file's own
// content, like an unrelated commit changing hunk boundaries — invalidates
// it, which is the conservative direction to be wrong in.
func fileDiffHash(fd FileDiff) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%v\x00", fd.Path, fd.Status, fd.Binary)
	for _, hunk := range fd.Hunks {
		fmt.Fprintf(h, "@%s\x00", hunk.Header)
		for _, l := range hunk.Lines {
			fmt.Fprintf(h, "%d\x00%s\x00%s\x00%s\x00", l.Kind, l.Content, intPtrStr(l.OldLine), intPtrStr(l.NewLine))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func intPtrStr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
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
			hunk := convertHunk(h)
			applyIntralineHighlights(&hunk)
			fd.Hunks = append(fd.Hunks, hunk)
		}
		files = append(files, fd)
	}
	return files, nil
}

// untrackedFilePaths lists files git isn't tracking and isn't ignoring
// (--exclude-standard honors .gitignore/.git/info/exclude/global excludes),
// via `git ls-files --others --exclude-standard` — read-only, unlike
// `git add -N`, which would mutate the index just to make these visible to
// `git diff`.
func untrackedFilePaths(runner GitRunner) ([]string, error) {
	out, err := runner.Run("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// looksBinary is git's own heuristic for "don't diff this as text": a NUL
// byte anywhere in the first chunk of the file. Checking only a prefix
// (matching git's own ~8000-byte sample) keeps this cheap for large files.
func looksBinary(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.IndexByte(data, 0) >= 0
}

// untrackedFileDiff synthesizes a FileDiff for a file git isn't tracking,
// so it can flow through the same rendering/commenting path as a real diff
// entry — status FileAdded, and (for a text file) a single hunk with every
// line shown as added, since from a fresh-file perspective the whole thing
// is new. A binary file gets Binary set instead of a hunk dump of raw bytes
// as "added lines" — see FileDiff.Binary.
func untrackedFileDiff(repoRoot, relPath string) (FileDiff, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, relPath))
	if err != nil {
		return FileDiff{}, err
	}
	fd := FileDiff{Path: relPath, Status: FileAdded, Untracked: true}
	if looksBinary(data) {
		fd.Binary = true
		return fd, nil
	}

	content := strings.Split(string(data), "\n")
	if len(content) > 0 && content[len(content)-1] == "" {
		content = content[:len(content)-1] // trailing newline, not a phantom empty final line
	}
	lines := make([]Line, len(content))
	for i, c := range content {
		n := i + 1
		lines[i] = Line{Kind: LineAdded, Content: c, NewLine: &n}
	}
	fd.Hunks = []Hunk{{Header: fmt.Sprintf("-0,0 +1,%d", len(lines)), Lines: lines}}
	return fd, nil
}

// untrackedFileDiffs is untrackedFilePaths plus untrackedFileDiff for each
// result — a file that vanishes or becomes unreadable between the two
// steps (a real possibility: nothing prevents the working tree from
// changing mid-scan) is skipped rather than failing the whole batch.
func untrackedFileDiffs(runner GitRunner, repoRoot string) ([]FileDiff, error) {
	paths, err := untrackedFilePaths(runner)
	if err != nil {
		return nil, err
	}
	files := make([]FileDiff, 0, len(paths))
	for _, p := range paths {
		fd, err := untrackedFileDiff(repoRoot, p)
		if err != nil {
			continue
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
