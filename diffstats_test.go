package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
	row := renderSidebarRow(fd, false, false, 0, 0, 40)
	if strings.Contains(row, "+1") || strings.Contains(row, "-1") {
		t.Fatalf("expected no per-file stats badge in the sidebar row, got %q", row)
	}
}

// TestSidebarRowShowsUnresolvedOverTotal guards the "unresolved/total"
// comment badge format — a fully-resolved file should still show its
// comment history (e.g. "(0/2)") rather than looking like it never had any.
func TestSidebarRowShowsUnresolvedOverTotal(t *testing.T) {
	fd := fileRows{file: FileDiff{Path: "a.go", Status: FileModified}}

	row := renderSidebarRow(fd, false, false, 2, 5, 40)
	if !strings.Contains(row, "(2/5)") {
		t.Fatalf("expected a %q badge, got %q", "(2/5)", row)
	}

	resolvedRow := renderSidebarRow(fd, false, false, 0, 2, 40)
	if !strings.Contains(resolvedRow, "(0/2)") {
		t.Fatalf("expected a fully-resolved file to still show %q, got %q", "(0/2)", resolvedRow)
	}

	noCommentsRow := renderSidebarRow(fd, false, false, 0, 0, 40)
	if strings.Contains(noCommentsRow, "(") {
		t.Fatalf("expected no badge at all when the file has no comments, got %q", noCommentsRow)
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

	header := ansi.Strip(m.renderHeader())
	if !strings.Contains(header, "+2 -0") {
		t.Fatalf("expected the selected file's own +2 -0 stat in the header, got %q", header)
	}
}
