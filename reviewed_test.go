package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFileDiffHashStableAndSensitiveToContent(t *testing.T) {
	a := fileDiffWithLines("a.go", 5)
	b := fileDiffWithLines("a.go", 5)
	if fileDiffHash(a) != fileDiffHash(b) {
		t.Fatal("expected identical FileDiffs to hash the same")
	}

	c := fileDiffWithLines("a.go", 6) // one more line
	if fileDiffHash(a) == fileDiffHash(c) {
		t.Fatal("expected different content to hash differently")
	}
}

// TestFileDiffHashDistinguishesUntracked guards setDiffFiles' flattenCache:
// an untracked file transitioning to tracked (same path/status/hunks) must
// not hash the same, or the cache would hand back the stale fileRows with
// the old Untracked value.
func TestFileDiffHashDistinguishesUntracked(t *testing.T) {
	tracked := fileDiffWithLines("a.go", 5)
	untracked := tracked
	untracked.Untracked = true
	if fileDiffHash(tracked) == fileDiffHash(untracked) {
		t.Fatal("expected Untracked to affect the hash")
	}
}

func TestToggleCurrentFileReviewedMarksAndUnmarks(t *testing.T) {
	withTempHome(t)
	fd := fileDiffWithLines("a.go", 5)
	m := newModel("/repo", []FileDiff{fd}, Session{}, nil)

	if isFileReviewed(m.session, flattenFile(fd)) {
		t.Fatal("expected not reviewed initially")
	}

	mm, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m2 := mm.(model)
	if !isFileReviewed(m2.session, flattenFile(fd)) {
		t.Fatal("expected reviewed after v")
	}

	mm, _ = m2.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m3 := mm.(model)
	if isFileReviewed(m3.session, flattenFile(fd)) {
		t.Fatal("expected un-reviewed after pressing v again")
	}
}

func TestFileChangeAutomaticallyUnreviewsIt(t *testing.T) {
	withTempHome(t)
	repo := "/repo"
	fd := fileDiffWithLines("a.go", 5)
	m := newModel(repo, []FileDiff{fd}, Session{}, nil)

	mm, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m2 := mm.(model)
	if !isFileReviewed(m2.session, flattenFile(fd)) {
		t.Fatal("expected reviewed")
	}

	// The file changes (more edits land) — same path, different content.
	changed := fileDiffWithLines("a.go", 9)
	if isFileReviewed(m2.session, flattenFile(changed)) {
		t.Fatal("expected a changed file to read as unreviewed even though the mark for that path still exists")
	}
}

func TestToggleReviewedPersistsAcrossReload(t *testing.T) {
	withTempHome(t)
	repo := "/repo"
	fd := fileDiffWithLines("a.go", 5)
	m := newModel(repo, []FileDiff{fd}, Session{}, nil)

	mm, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m2 := mm.(model)
	if !isFileReviewed(m2.session, flattenFile(fd)) {
		t.Fatal("expected reviewed")
	}

	reloaded, err := loadSession(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !isFileReviewed(reloaded, flattenFile(fd)) {
		t.Fatal("expected the reviewed mark to survive a session reload")
	}
}
