package main

import "testing"

func TestSidebarWidthShrinksForShortPaths(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 3),
		fileDiffWithLines("b.go", 3),
	}, Session{}, nil)

	if w := m.sidebarWidth(); w != minSidebarWidth {
		t.Fatalf("expected the floor (%d) for short paths, got %d", minSidebarWidth, w)
	}
}

func TestSidebarWidthCapsForLongPaths(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("internal/some/very/deeply/nested/package/file.go", 3),
	}, Session{}, nil)

	if w := m.sidebarWidth(); w != maxSidebarWidth {
		t.Fatalf("expected the cap (%d) for a long path, got %d", maxSidebarWidth, w)
	}
}

func TestSidebarWidthTracksLongestPathAmongAllFiles(t *testing.T) {
	short := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)
	longer := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 3),
		fileDiffWithLines("src/services/lockers/column_sizes.go", 3),
	}, Session{}, nil)

	if longer.sidebarWidth() <= short.sidebarWidth() {
		t.Fatalf("expected adding a longer path to widen the sidebar: short=%d longer=%d",
			short.sidebarWidth(), longer.sidebarWidth())
	}
}

// TestSidebarWidthGrowsPastCapOnWideTerminal guards the fix for a long file
// path truncating even when the terminal has plenty of room to show it —
// maxSidebarWidth was sized for a typical terminal, but on a wide one the
// cap should grow (up to sidebarWidthFraction of the total width) instead
// of staying stuck at that static value.
func TestSidebarWidthGrowsPastCapOnWideTerminal(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("internal/some/very/deeply/nested/package/quite/a/lot/longer/file.go", 3),
	}, Session{}, nil)

	m.width = 80
	narrow := m.sidebarWidth()
	if narrow != maxSidebarWidth {
		t.Fatalf("expected a normal-width terminal to still hit the static cap (%d), got %d", maxSidebarWidth, narrow)
	}

	m.width = 300
	wide := m.sidebarWidth()
	if wide <= maxSidebarWidth {
		t.Fatalf("expected a wide terminal to grow the sidebar past the static cap (%d), got %d", maxSidebarWidth, wide)
	}
}

func TestSidebarWidthIgnoresActiveFilterWhenSizing(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 3),
		fileDiffWithLines("src/services/lockers/column_sizes.go", 3),
	}, Session{}, nil)
	before := m.sidebarWidth()

	m.fileFilter = "a.go" // filters down to just the short one
	after := m.sidebarWidth()

	if before != after {
		t.Fatalf("expected sidebar width to stay stable across a filter change (sized off all files, not just visible ones): before=%d after=%d", before, after)
	}
}
