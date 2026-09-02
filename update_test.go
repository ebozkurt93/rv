package main

import (
	"testing"
	"time"
)

func TestDeleteWordFromEnd(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello world", "hello "},
		{"hello world  ", "hello "},
		{"hello", ""},
		{"", ""},
		{"   ", ""},
		{"fix the bug\n", "fix the "},
		{"a", ""},
	}
	for _, c := range cases {
		if got := deleteWordFromEnd(c.in); got != c.want {
			t.Errorf("deleteWordFromEnd(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDeleteWordFromEndRepeatedPressesClearTheLine guards the actual UX:
// pressing it repeatedly should walk all the way back to empty, one word at
// a time, the way it does in a shell or editor.
func TestDeleteWordFromEndRepeatedPressesClearTheLine(t *testing.T) {
	s := "fix the flaky test"
	for i := 0; i < 10 && s != ""; i++ {
		s = deleteWordFromEnd(s)
	}
	if s != "" {
		t.Fatalf("expected repeated word-deletes to empty the string, got %q", s)
	}
}

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

func TestJumpToHunkCrossesFilesAtEitherEnd(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithHunks("a.go", 2, 2), // rows: 0=hdr,1-2,3=hdr,4-5
		fileDiffWithHunks("b.go", 2),    // rows: 0=hdr,1-2
	}, Session{}, nil)
	// starts on a.go's first hunk's first line (row 1, per the cursor
	// invariant — never on a header)

	m.jumpToHunk(1) // -> a.go's 2nd hunk
	if m.fileIndex != 0 || m.lineIndex != 4 {
		t.Fatalf("expected a.go row 4, got file=%d line=%d", m.fileIndex, m.lineIndex)
	}

	m.jumpToHunk(1) // a.go has no more hunks -> falls through to b.go
	if m.fileIndex != 1 || m.lineIndex != 1 {
		t.Fatalf("expected to cross into b.go at row 1, got file=%d line=%d", m.fileIndex, m.lineIndex)
	}

	m.jumpToHunk(1) // b.go has no more hunks either -> wraps back to a.go's first hunk
	if m.fileIndex != 0 || m.lineIndex != 1 {
		t.Fatalf("expected to wrap back to a.go row 1, got file=%d line=%d", m.fileIndex, m.lineIndex)
	}

	m.jumpToHunk(-1) // backward across files -> lands on b.go's last hunk's first line
	if m.fileIndex != 1 || m.lineIndex != 1 {
		t.Fatalf("expected to cross backward into b.go row 1, got file=%d line=%d", m.fileIndex, m.lineIndex)
	}
}

func TestJumpToHunkAcrossFilesNoOpWithOnlyOneFile(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 2)}, Session{}, nil)
	before := m.lineIndex
	m.jumpToHunk(1)
	if m.lineIndex != before || m.fileIndex != 0 {
		t.Fatalf("expected no-op with a single file, got file=%d line=%d", m.fileIndex, m.lineIndex)
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

// TestJumpToCommentIncludesResolvedWhenScopeToggled guards keys.
// ToggleCommentScope — n/N skip resolved comments by default, but include
// them once commentNavIncludeResolved is on.
func TestJumpToCommentIncludesResolvedWhenScopeToggled(t *testing.T) {
	withTempHome(t)
	n1 := 1
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{Comments: []Comment{
		{ID: "c_a", File: "a.go", NewLine: &n1, Resolved: true},
	}}, nil)
	m.fileIndex, m.lineIndex = 0, 2 // past the only (resolved) comment

	m.jumpToComment(1)
	if got, _ := m.currentLine(); got.NewLine != nil && *got.NewLine == n1 {
		t.Fatalf("expected the resolved comment to still be skipped by default")
	}

	m.commentNavIncludeResolved = true
	m.jumpToComment(1)
	got, ok := m.currentLine()
	if !ok || got.NewLine == nil || *got.NewLine != n1 {
		t.Fatalf("expected the resolved comment reachable once scope includes resolved, got %+v ok=%v", got, ok)
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
