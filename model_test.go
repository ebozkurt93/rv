package main

import "testing"

func fileDiffWithLines(path string, n int) FileDiff {
	var lines []Line
	for i := 1; i <= n; i++ {
		ln := i
		lines = append(lines, Line{Kind: LineContext, Content: "x", OldLine: &ln, NewLine: &ln})
	}
	return FileDiff{Path: path, Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: lines}}}
}

func TestSetDiffFilesKeepsSelectionOnSamePath(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 5),
		fileDiffWithLines("b.go", 5),
	}, Session{})
	m.selectFile(1) // b.go
	m.lineIndex = 3

	// Reorder + add a file; b.go should still be selected regardless of its
	// new position, with lineIndex preserved since it still fits.
	m.setDiffFiles([]FileDiff{
		fileDiffWithLines("z.go", 5),
		fileDiffWithLines("a.go", 5),
		fileDiffWithLines("b.go", 5),
	})

	if m.files[m.fileIndex].file.Path != "b.go" {
		t.Fatalf("expected selection to stay on b.go, got %q", m.files[m.fileIndex].file.Path)
	}
	if m.lineIndex != 3 {
		t.Fatalf("expected lineIndex preserved at 3, got %d", m.lineIndex)
	}
}

func TestSetDiffFilesClampsLineIndexWhenFileShrinks(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 10)}, Session{})
	m.lineIndex = 9

	m.setDiffFiles([]FileDiff{fileDiffWithLines("a.go", 3)})

	// fileDiffWithLines produces 1 hunk-header row + n line rows, so the
	// last valid index for n=3 is 3 (row 0 is the header).
	if m.lineIndex != 3 {
		t.Fatalf("expected lineIndex clamped to 3 (last row), got %d", m.lineIndex)
	}
}

func TestSetDiffFilesFallsBackWhenFileDisappears(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 5),
		fileDiffWithLines("b.go", 5),
	}, Session{})
	m.selectFile(1) // b.go

	m.setDiffFiles([]FileDiff{fileDiffWithLines("a.go", 5)})

	if m.fileIndex != 0 || m.files[m.fileIndex].file.Path != "a.go" {
		t.Fatalf("expected fallback to the only remaining file, got index %d", m.fileIndex)
	}
}

func TestSetDiffFilesEmptyResultResetsCursor(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 5)}, Session{})
	m.setDiffFiles(nil)

	if len(m.files) != 0 || m.lineIndex != 0 {
		t.Fatalf("expected empty state, got files=%d lineIndex=%d", len(m.files), m.lineIndex)
	}
}
