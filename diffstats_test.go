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

// TestHeaderRowsShareLeftMargin guards the fix for header/footer text
// sitting flush against the terminal edge while the panels below it are
// inset by their border+padding — every header row (and the footer) should
// start with the same headerMargin of leading spaces.
func TestHeaderRowsShareLeftMargin(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{
		{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineAdded, Content: "x", NewLine: intp(1)},
		}}}},
	}, Session{}, nil)
	m.width, m.height = 100, 24

	margin := strings.Repeat(" ", headerMargin)
	rows := strings.Split(ansi.Strip(m.renderHeader()), "\n")
	if len(rows) != 3 {
		t.Fatalf("expected 3 header rows (title/rule/path), got %d: %q", len(rows), rows)
	}
	if !strings.HasPrefix(rows[0], margin) {
		t.Fatalf("expected the title row to start with %d-space margin, got %q", headerMargin, rows[0])
	}
	if !strings.HasPrefix(rows[2], margin) {
		t.Fatalf("expected the path row to start with %d-space margin, got %q", headerMargin, rows[2])
	}
	if !strings.HasSuffix(rows[0], margin) {
		t.Fatalf("expected the title row to end with %d-space margin, got %q", headerMargin, rows[0])
	}

	footer := ansi.Strip(m.renderFooter())
	if !strings.HasPrefix(footer, margin) {
		t.Fatalf("expected the footer to start with %d-space margin, got %q", headerMargin, footer)
	}
}

// TestHeaderPathLineShowsReviewedCheckAndCount guards the two additions to
// the path line: a reviewed checkmark (mirroring the sidebar's own), and
// an "N/M reviewed" count on the right when there's room for it.
func TestHeaderPathLineShowsReviewedCheckAndCount(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{
		{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineAdded, Content: "x", NewLine: intp(1)},
		}}}},
		{Path: "b.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineAdded, Content: "y", NewLine: intp(1)},
		}}}},
	}, Session{}, nil)
	m.width, m.height = 100, 24

	before := ansi.Strip(m.renderHeader())
	if strings.Contains(before, "✓ a.go") {
		t.Fatalf("expected no checkmark before marking reviewed, got %q", before)
	}
	if !strings.Contains(before, "0/2 reviewed") {
		t.Fatalf("expected a 0/2 reviewed count with room at width 100, got %q", before)
	}

	m.toggleCurrentFileReviewed()
	after := ansi.Strip(m.renderHeader())
	if !strings.Contains(after, "✓ a.go") {
		t.Fatalf("expected a checkmark on the path line once reviewed, got %q", after)
	}
	if !strings.Contains(after, "1/2 reviewed") {
		t.Fatalf("expected the count to update to 1/2, got %q", after)
	}
}

// TestHeaderPathLineDropsReviewedCountWhenNarrow guards the "if we have
// the space" part: a narrow terminal should keep the path readable rather
// than squeeze in the reviewed count.
func TestHeaderPathLineDropsReviewedCountWhenNarrow(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{
		{Path: "internal/some/very/deeply/nested/package/file.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineAdded, Content: "x", NewLine: intp(1)},
		}}}},
	}, Session{}, nil)
	m.width, m.height = 40, 24

	header := ansi.Strip(m.renderHeader())
	rows := strings.Split(header, "\n")
	pathRow := rows[len(rows)-1]
	if strings.Contains(pathRow, "1/1 reviewed") {
		t.Fatalf("expected the path row to drop its reviewed count at width 40 to keep the path readable, got %q", pathRow)
	}
}
