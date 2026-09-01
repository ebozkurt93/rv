package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func threeSimpleFilesModel() model {
	m := newModel("/repo", []FileDiff{
		fileDiffWithLines("a.go", 3),
		fileDiffWithLines("b.go", 3),
		fileDiffWithLines("c.go", 3),
	}, Session{}, nil)
	m.width, m.height = 100, 24
	return m
}

// Coordinates below (header=3 lines, sidebar top border at y=3, rows at
// y=4,5,6 for a.go/b.go/c.go) are verified against a real rendered frame in
// this exact configuration — see the layout probed while building this.

func TestClickSidebarRowSelectsThatFile(t *testing.T) {
	m := threeSimpleFilesModel()

	mm, _ := m.Update(tea.MouseMsg{X: 5, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m2 := mm.(model)
	if m2.files[m2.fileIndex].file.Path != "b.go" {
		t.Fatalf("expected clicking row y=5 to select b.go, got %q", m2.files[m2.fileIndex].file.Path)
	}

	mm, _ = m.Update(tea.MouseMsg{X: 5, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m3 := mm.(model)
	if m3.files[m3.fileIndex].file.Path != "c.go" {
		t.Fatalf("expected clicking row y=6 to select c.go, got %q", m3.files[m3.fileIndex].file.Path)
	}
}

func TestClickOutsideSidebarRowsIsIgnored(t *testing.T) {
	m := threeSimpleFilesModel()
	before := m.fileIndex

	// y=3 is the sidebar's top border, not a file row.
	mm, _ := m.Update(tea.MouseMsg{X: 5, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m2 := mm.(model)
	if m2.fileIndex != before {
		t.Fatalf("expected clicking the border to be a no-op, selection changed to %d", m2.fileIndex)
	}
}

func TestClickInDiffPaneDoesNotChangeFileSelection(t *testing.T) {
	m := threeSimpleFilesModel()
	before := m.fileIndex

	mm, _ := m.Update(tea.MouseMsg{X: 60, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m2 := mm.(model)
	if m2.fileIndex != before {
		t.Fatalf("expected a click in the diff pane to leave file selection alone, got %d", m2.fileIndex)
	}
}

func TestWheelOverSidebarChangesFileSelection(t *testing.T) {
	m := threeSimpleFilesModel() // starts on a.go

	mm, _ := m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown})
	m2 := mm.(model)
	if m2.files[m2.fileIndex].file.Path != "b.go" {
		t.Fatalf("expected wheel-down over sidebar to move to b.go, got %q", m2.files[m2.fileIndex].file.Path)
	}

	mm, _ = m2.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelUp})
	m3 := mm.(model)
	if m3.files[m3.fileIndex].file.Path != "a.go" {
		t.Fatalf("expected wheel-up over sidebar to move back to a.go, got %q", m3.files[m3.fileIndex].file.Path)
	}
}

func TestWheelOverDiffPaneMovesCursorNotFile(t *testing.T) {
	m := threeSimpleFilesModel()
	beforeFile := m.fileIndex
	beforeLine := m.lineIndex

	mm, _ := m.Update(tea.MouseMsg{X: 60, Y: 5, Button: tea.MouseButtonWheelDown})
	m2 := mm.(model)
	if m2.fileIndex != beforeFile {
		t.Fatalf("expected wheel over diff pane to leave file selection alone, got %d", m2.fileIndex)
	}
	if m2.lineIndex == beforeLine {
		t.Fatalf("expected wheel over diff pane to move the cursor, stayed at %d", m2.lineIndex)
	}
}

func TestMouseIgnoredOutsideNormalMode(t *testing.T) {
	m := threeSimpleFilesModel()
	m.mode = modeHelp
	before := m.fileIndex

	mm, _ := m.Update(tea.MouseMsg{X: 5, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m2 := mm.(model)
	if m2.fileIndex != before || m2.mode != modeHelp {
		t.Fatalf("expected mouse clicks to be ignored while help is open, got fileIndex=%d mode=%v", m2.fileIndex, m2.mode)
	}
}

func TestClickSidebarRespectsScroll(t *testing.T) {
	// More files than fit on screen, selection deep in the list so the
	// sidebar has scrolled — a click must map through the same scroll the
	// frame was actually drawn with, not row 0's naive offset.
	var files []FileDiff
	for i := 0; i < 30; i++ {
		files = append(files, fileDiffWithLines(string(rune('a'+i))+".go", 2))
	}
	m := newModel("/repo", files, Session{}, nil)
	m.width, m.height = 100, 24
	m.selectFile(20) // scroll the sidebar down

	innerH := m.bodyHeight() - borderOverheadH
	vis := m.visibleFileIndices(m.fileFilter)
	scroll := clampScroll(20, len(vis), innerH)

	// Click the row that should now show file index `scroll` (the top of
	// the visible window).
	y := m.headerHeight() + 1 + 0
	mm, _ := m.Update(tea.MouseMsg{X: 5, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m2 := mm.(model)
	if m2.fileIndex != scroll {
		t.Fatalf("expected clicking the top visible row to select file index %d, got %d", scroll, m2.fileIndex)
	}
}
