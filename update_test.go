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

func TestJumpToHunkForwardAndBackward(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 2, 3, 1)}, Session{}, nil)
	// rows: [0]=hunk1 header [1,2]=lines [3]=hunk2 header [4,5,6]=lines [7]=hunk3 header [8]=line
	m.lineIndex = 0

	m.jumpToHunk(1)
	if m.lineIndex != 4 {
		t.Fatalf("expected jump to hunk 2's first line at row 4, got %d", m.lineIndex)
	}
	m.jumpToHunk(1)
	if m.lineIndex != 8 {
		t.Fatalf("expected jump to hunk 3's first line at row 8, got %d", m.lineIndex)
	}
	m.jumpToHunk(1) // no more hunks forward — should stay put
	if m.lineIndex != 8 {
		t.Fatalf("expected no-op past the last hunk, got %d", m.lineIndex)
	}

	m.jumpToHunk(-1)
	if m.lineIndex != 4 {
		t.Fatalf("expected jump back to hunk 2's first line at row 4, got %d", m.lineIndex)
	}
}

func TestJumpToCommentSkipsResolvedAndWraps(t *testing.T) {
	n1, n2, n3 := 1, 1, 1
	files := []FileDiff{
		fileDiffWithLines("a.go", 3),
		fileDiffWithLines("b.go", 3),
	}
	session := Session{Comments: []Comment{
		{ID: "c_a", File: "a.go", NewLine: &n1, Resolved: true}, // skipped
		{ID: "c_b", File: "a.go", NewLine: &n2},
		{ID: "c_c", File: "b.go", NewLine: &n3},
	}}
	m := newModel("/repo", files, session, nil)
	m.fileIndex, m.lineIndex = 0, 0 // before c_b

	m.jumpToComment(1)
	if m.fileIndex != 0 || m.files[0].file.Path != "a.go" {
		t.Fatalf("expected to land on a.go (c_b), got fileIndex=%d", m.fileIndex)
	}
	gotLine, _ := m.currentLine()
	if gotLine.NewLine == nil || *gotLine.NewLine != n2 {
		t.Fatalf("expected cursor on line %d, got %+v", n2, gotLine)
	}

	m.jumpToComment(1)
	if m.files[m.fileIndex].file.Path != "b.go" {
		t.Fatalf("expected to advance to b.go (c_c), got %q", m.files[m.fileIndex].file.Path)
	}

	m.jumpToComment(1) // wraps back around
	if m.files[m.fileIndex].file.Path != "a.go" {
		t.Fatalf("expected wraparound back to a.go, got %q", m.files[m.fileIndex].file.Path)
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
