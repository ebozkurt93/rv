package main

import (
	"strings"
	"testing"
)

func TestDiffStatsTotalsAcrossAllFiles(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 3), // 3 context lines, no +/-
		{
			Path: "b.go", Status: FileModified,
			Hunks: []Hunk{{Header: "h", Lines: []Line{
				{Kind: LineAdded, Content: "x", NewLine: intp(1)},
				{Kind: LineAdded, Content: "y", NewLine: intp(2)},
				{Kind: LineRemoved, Content: "z", OldLine: intp(1)},
			}}},
		},
	}, Session{}, nil)

	added, removed := m.diffStats()
	if added != 2 || removed != 1 {
		t.Fatalf("expected +2/-1, got +%d/-%d", added, removed)
	}
}

func TestFileDiffStatsPerFile(t *testing.T) {
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineAdded, Content: "x", NewLine: intp(1)},
		{Kind: LineAdded, Content: "y", NewLine: intp(2)},
		{Kind: LineRemoved, Content: "z", OldLine: intp(1)},
	}}}}
	added, removed := fileDiffStats(fd)
	if added != 2 || removed != 1 {
		t.Fatalf("expected +2/-1, got +%d/-%d", added, removed)
	}
}

// The sidebar deliberately does NOT show per-file +/- stats (removed after
// trying it — too noisy per row); it lives only in the header, both as a
// whole-diff total and for whichever file is currently selected.
func TestSidebarRowDoesNotShowPerFileStats(t *testing.T) {
	fd := fileRows{file: FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineAdded, Content: "x", NewLine: intp(1)},
		{Kind: LineRemoved, Content: "z", OldLine: intp(1)},
	}}}}}
	row := renderSidebarRow(fd, false, false, 0, 40)
	if strings.Contains(row, "+1") || strings.Contains(row, "-1") {
		t.Fatalf("expected no per-file stats badge in the sidebar row, got %q", row)
	}
}

func TestHeaderShowsDiffStats(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{
		{
			Path: "b.go", Status: FileModified,
			Hunks: []Hunk{{Header: "h", Lines: []Line{
				{Kind: LineAdded, Content: "x", NewLine: intp(1)},
				{Kind: LineRemoved, Content: "z", OldLine: intp(1)},
			}}},
		},
	}, Session{}, nil)
	m.width, m.height = 100, 24

	header := m.renderHeader()
	if !strings.Contains(header, "+1") || !strings.Contains(header, "-1") {
		t.Fatalf("expected header to show +1 -1, got %q", header)
	}
}

func TestHeaderPathLineShowsSelectedFileStats(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{
		{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineAdded, Content: "x", NewLine: intp(1)},
			{Kind: LineAdded, Content: "y", NewLine: intp(2)},
		}}}},
	}, Session{}, nil)
	m.width, m.height = 100, 24

	header := m.renderHeader()
	if !strings.Contains(header, "+2 -0") {
		t.Fatalf("expected the selected file's own +2 -0 stat in the header, got %q", header)
	}
}
