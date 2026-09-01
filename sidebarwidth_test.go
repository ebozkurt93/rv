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
