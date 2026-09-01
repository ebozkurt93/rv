package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected, returning what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error: %v", err)
	}
	prev := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = prev
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return string(out)
}

func TestRunCommentResolveMarksResolved(t *testing.T) {
	withTempHome(t)
	repo := "/repo/one"
	n := 1
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "foo.go", NewLine: &n, Body: "fix this"},
	}}))

	withRepoRoot(t, repo, func() {
		if err := runCommentResolve([]string{"c_1"}); err != nil {
			t.Fatalf("runCommentResolve error: %v", err)
		}
	})

	s, err := loadSession(repo)
	if err != nil {
		t.Fatalf("loadSession error: %v", err)
	}
	if !s.Comments[0].Resolved {
		t.Fatalf("expected comment resolved, got %+v", s.Comments[0])
	}
}

func TestRunCommentResolveUnknownID(t *testing.T) {
	withTempHome(t)
	repo := "/repo/one"
	must(t, saveSession(repo, Session{RepoRoot: repo}))

	withRepoRoot(t, repo, func() {
		if err := runCommentResolve([]string{"c_missing"}); err == nil {
			t.Fatalf("expected error for unknown comment id")
		}
	})
}

func TestRunCommentReplyAppendsAndCanResolve(t *testing.T) {
	withTempHome(t)
	repo := "/repo/one"
	n := 1
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "foo.go", NewLine: &n, Body: "fix this"},
	}}))

	withRepoRoot(t, repo, func() {
		err := runCommentReply([]string{"--resolve", "c_1", "done,", "fixed", "it"})
		if err != nil {
			t.Fatalf("runCommentReply error: %v", err)
		}
	})

	s, err := loadSession(repo)
	if err != nil {
		t.Fatalf("loadSession error: %v", err)
	}
	c := s.Comments[0]
	if !c.Resolved {
		t.Fatalf("expected --resolve to mark the comment resolved")
	}
	if len(c.Replies) != 1 || c.Replies[0].Body != "done, fixed it" || c.Replies[0].Author != "agent" {
		t.Fatalf("unexpected replies: %+v", c.Replies)
	}
}

// TestRunCommentReplyResolveFlagAfterPositionalArgs guards against a real
// bug found by hand: flag.FlagSet stops recognizing flags at the first
// positional argument, and here the id (a positional arg) necessarily comes
// before the reply text, so `reply <id> <text> --resolve` — a completely
// natural way for an agent to invoke this — silently swallowed "--resolve"
// into the reply body instead of applying it.
func TestRunCommentReplyResolveFlagAfterPositionalArgs(t *testing.T) {
	withTempHome(t)
	repo := "/repo/one"
	n := 1
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "foo.go", NewLine: &n, Body: "fix this"},
	}}))

	withRepoRoot(t, repo, func() {
		err := runCommentReply([]string{"c_1", "done,", "fixed", "it", "--resolve"})
		if err != nil {
			t.Fatalf("runCommentReply error: %v", err)
		}
	})

	s, err := loadSession(repo)
	if err != nil {
		t.Fatalf("loadSession error: %v", err)
	}
	c := s.Comments[0]
	if !c.Resolved {
		t.Fatalf("expected trailing --resolve to still mark the comment resolved")
	}
	if len(c.Replies) != 1 || c.Replies[0].Body != "done, fixed it" {
		t.Fatalf("expected --resolve stripped from the reply body, got %+v", c.Replies)
	}
}

func TestFilterCommentsByFileAndUnresolved(t *testing.T) {
	n := 1
	comments := []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Resolved: true},
		{ID: "c_2", File: "a.go", NewLine: &n},
		{ID: "c_3", File: "b.go", NewLine: &n},
	}

	got := filterComments(comments, "a.go", false)
	if len(got) != 2 {
		t.Fatalf("expected 2 comments for a.go, got %d", len(got))
	}

	got = filterComments(comments, "a.go", true)
	if len(got) != 1 || got[0].ID != "c_2" {
		t.Fatalf("expected only unresolved c_2, got %+v", got)
	}

	got = filterComments(comments, "", false)
	if len(got) != 3 {
		t.Fatalf("expected all 3 comments with no filter, got %d", len(got))
	}
}

func TestPrintCommentsTextIncludesReplies(t *testing.T) {
	n := 1
	comments := []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Author: "user", Body: "please fix",
			Replies: []Reply{{Author: "agent", Body: "fixed"}}},
	}
	out := captureStdout(t, func() { printCommentsText(comments) })
	if !strings.Contains(out, "please fix") || !strings.Contains(out, "agent: fixed") {
		t.Fatalf("unexpected output: %q", out)
	}
}

// withRepoRoot points currentRepoRoot() at repo for the duration of fn by
// faking the git runner's `rev-parse --show-toplevel` response.
func withRepoRoot(t *testing.T, repo string, fn func()) {
	t.Helper()
	prev := gitRunner
	gitRunner = &fakeGitRunner{output: repo + "\n"}
	t.Cleanup(func() { gitRunner = prev })
	fn()
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
