package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func threeFileModel() model {
	return newModel("/repo", []FileDiff{
		fileDiffWithLines("src/foo.go", 3),
		fileDiffWithLines("src/bar.go", 3),
		fileDiffWithLines("docs/readme.md", 3),
	}, Session{}, nil)
}

func TestVisibleFileIndicesFiltersCaseInsensitiveSubstring(t *testing.T) {
	m := threeFileModel()

	got := m.visibleFileIndices("FOO")
	if len(got) != 1 || m.files[got[0]].file.Path != "src/foo.go" {
		t.Fatalf("expected only foo.go, got %v", got)
	}

	got = m.visibleFileIndices("src/")
	if len(got) != 2 {
		t.Fatalf("expected 2 files under src/, got %d", len(got))
	}

	got = m.visibleFileIndices("")
	if len(got) != 3 {
		t.Fatalf("expected all 3 files with no filter, got %d", len(got))
	}

	got = m.visibleFileIndices("nonexistent")
	if len(got) != 0 {
		t.Fatalf("expected no matches, got %v", got)
	}
}

func TestSlashOpensSearchPrefillingCurrentFilter(t *testing.T) {
	m := threeFileModel()
	m.fileFilter = "foo"

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2 := mm.(model)
	if m2.mode != modeSearch {
		t.Fatalf("expected modeSearch, got %v", m2.mode)
	}
	if m2.input != "foo" {
		t.Fatalf("expected search input prefilled with existing filter, got %q", m2.input)
	}
}

func TestSearchConfirmAppliesFilterAndSnapsSelection(t *testing.T) {
	m := threeFileModel()
	m.fileIndex = 2 // docs/readme.md

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2 := mm.(model)
	for _, r := range "bar" {
		mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m2 = mm.(model)
	}
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := mm.(model)

	if m3.mode != modeNormal {
		t.Fatalf("expected Confirm to return to modeNormal, got %v", m3.mode)
	}
	if m3.fileFilter != "bar" {
		t.Fatalf("expected fileFilter=%q, got %q", "bar", m3.fileFilter)
	}
	if m3.files[m3.fileIndex].file.Path != "src/bar.go" {
		t.Fatalf("expected selection snapped to src/bar.go (only match), got %q", m3.files[m3.fileIndex].file.Path)
	}
}

func TestSearchCancelLeavesFilterUnchanged(t *testing.T) {
	m := threeFileModel()
	m.fileFilter = "foo"

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2 := mm.(model)
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2 = mm.(model)
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m3 := mm.(model)

	if m3.mode != modeNormal {
		t.Fatalf("expected Cancel to return to modeNormal, got %v", m3.mode)
	}
	if m3.fileFilter != "foo" {
		t.Fatalf("expected fileFilter left untouched at %q, got %q", "foo", m3.fileFilter)
	}
}

func TestSelectVisibleFileSkipsFilteredOutEntries(t *testing.T) {
	m := threeFileModel()
	m.fileFilter = "src/" // matches foo.go (0) and bar.go (1), not readme.md (2)
	m.fileIndex = 0

	m.selectVisibleFile(1)
	if m.fileIndex != 1 {
		t.Fatalf("expected to move to bar.go (index 1), got %d", m.fileIndex)
	}

	m.selectVisibleFile(1) // past the last visible file — wraps to the first
	if m.fileIndex != 0 {
		t.Fatalf("expected to wrap back to foo.go (index 0), got %d", m.fileIndex)
	}
}

func TestSnapToVisibleFileMovesOffFilteredSelection(t *testing.T) {
	m := threeFileModel()
	m.fileIndex = 2 // docs/readme.md, about to be filtered out
	m.fileFilter = "src/"

	m.snapToVisibleFile()

	if m.fileIndex == 2 {
		t.Fatalf("expected selection to move off the filtered-out file")
	}
	if m.files[m.fileIndex].file.Path != "src/foo.go" && m.files[m.fileIndex].file.Path != "src/bar.go" {
		t.Fatalf("expected selection on a visible file, got %q", m.files[m.fileIndex].file.Path)
	}
}
