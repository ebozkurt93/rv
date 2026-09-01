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

// fileDiffWithHunks builds a file with len(hunkSizes) hunks, each with the
// given number of context lines, for tests that need multiple hunks to jump
// between.
func fileDiffWithHunks(path string, hunkSizes ...int) FileDiff {
	fd := FileDiff{Path: path, Status: FileModified}
	line := 1
	for _, n := range hunkSizes {
		var lines []Line
		for i := 0; i < n; i++ {
			ln := line
			lines = append(lines, Line{Kind: LineContext, Content: "x", OldLine: &ln, NewLine: &ln})
			line++
		}
		fd.Hunks = append(fd.Hunks, Hunk{Header: "h", Lines: lines})
	}
	return fd
}

func TestSetDiffFilesKeepsSelectionOnSamePath(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 5),
		fileDiffWithLines("b.go", 5),
	}, Session{}, nil)
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
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 10)}, Session{}, nil)
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
	}, Session{}, nil)
	m.selectFile(1) // b.go

	m.setDiffFiles([]FileDiff{fileDiffWithLines("a.go", 5)})

	if m.fileIndex != 0 || m.files[m.fileIndex].file.Path != "a.go" {
		t.Fatalf("expected fallback to the only remaining file, got index %d", m.fileIndex)
	}
}

func TestSetDiffFilesEmptyResultResetsCursor(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 5)}, Session{}, nil)
	m.setDiffFiles(nil)

	if len(m.files) != 0 || m.lineIndex != 0 {
		t.Fatalf("expected empty state, got files=%d lineIndex=%d", len(m.files), m.lineIndex)
	}
}

func TestCurrentLineLabel(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 3, 5)}, Session{}, nil)

	// The cursor invariant (never rest on a hunk header) means a freshly
	// opened file already starts on its first content line, not row 0.
	if got := m.currentLineLabel(); got != "1" {
		t.Fatalf("expected '1' (cursor should never start on the header row), got %q", got)
	}

	// rows: 0=hdr,1-3=lines(1-3),4=hdr,5-9=lines(4-8)
	m.lineIndex = 5
	if got := m.currentLineLabel(); got != "4" {
		t.Fatalf("expected '4' (first line of the 2nd hunk), got %q", got)
	}

	// currentLineLabel's "hunk" fallback is defensive — the invariant means
	// no normal code path should ever actually land lineIndex on a header,
	// but if one slipped through, it should say so rather than panic or
	// silently return a wrong number.
	m.lineIndex = 4
	if got := m.currentLineLabel(); got != "hunk" {
		t.Fatalf("expected the defensive 'hunk' fallback, got %q", got)
	}
}
