package main

import (
	"testing"
	"time"
)

func TestRefreshSessionIfChangedPicksUpExternalWrite(t *testing.T) {
	withTempHome(t)
	repo := "/repo/one"

	m := newModel(repo, nil, Session{RepoRoot: repo}, nil)

	// Simulate an agent replying via `rv comment reply` in another process:
	// a direct write to the session file, bypassing this model entirely.
	n := 1
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "foo.go", NewLine: &n, Body: "hi"},
	}}))

	m.refreshSessionIfChanged()

	if len(m.session.Comments) != 1 || m.session.Comments[0].ID != "c_1" {
		t.Fatalf("expected external write to be picked up, got %+v", m.session)
	}
}

func TestRefreshSessionIfChangedNoOpWhenUnchanged(t *testing.T) {
	withTempHome(t)
	repo := "/repo/one"
	must(t, saveSession(repo, Session{RepoRoot: repo}))

	m := newModel(repo, nil, Session{RepoRoot: repo}, nil)
	m.session.Comments = []Comment{{ID: "local-only"}} // not persisted

	m.refreshSessionIfChanged()

	if len(m.session.Comments) != 1 || m.session.Comments[0].ID != "local-only" {
		t.Fatalf("expected no reload since the file's mtime hasn't advanced, got %+v", m.session)
	}
}

func TestSessionModTimeZeroWhenMissing(t *testing.T) {
	withTempHome(t)
	mt, err := sessionModTime("/repo/never-written")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mt.IsZero() {
		t.Fatalf("expected zero time for a session that was never written, got %v", mt)
	}
	if !time.Now().After(mt) {
		t.Fatalf("sanity check failed: now should be after the zero time")
	}
}
