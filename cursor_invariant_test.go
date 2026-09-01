package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// assertNotOnHeader is the invariant every test in this file checks: the
// cursor must never rest on a rowHunkHeader row.
func assertNotOnHeader(t *testing.T, m model, where string) {
	t.Helper()
	rows := m.currentRows()
	if m.lineIndex < 0 || m.lineIndex >= len(rows) {
		return
	}
	if rows[m.lineIndex].kind == rowHunkHeader {
		t.Fatalf("%s: cursor landed on a hunk-header row (lineIndex=%d)", where, m.lineIndex)
	}
}

func TestCursorNeverStartsOnHeader(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 3, 5)}, Session{}, nil)
	assertNotOnHeader(t, m, "newModel")
}

func TestCursorNeverOnHeaderAfterGGOrG(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 3, 5)}, Session{}, nil)

	m.jumpTo(false, 0, false) // gg, no count
	assertNotOnHeader(t, m, "gg")

	m.jumpTo(false, 0, true) // G, no count
	assertNotOnHeader(t, m, "G")

	// {count}gg / {count}G landing exactly on a header's row index (0 or 4)
	m.jumpTo(true, 1, false) // "1gg" -> row index 0, which is a header
	assertNotOnHeader(t, m, "1gg")

	m.jumpTo(true, 5, true) // "5G" -> row index 4, which is a header
	assertNotOnHeader(t, m, "5G")
}

func TestCursorNeverOnHeaderSteppingWithMoveCursor(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 2, 2, 2)}, Session{}, nil)
	// rows: 0=hdr,1-2=lines,3=hdr,4-5=lines,6=hdr,7-8=lines
	for step := -1; step <= 1; step += 2 {
		m2 := m
		for i := 0; i < 10; i++ {
			m2.moveCursor(step)
			assertNotOnHeader(t, m2, "moveCursor step-by-step")
		}
	}
}

func TestCursorNeverOnHeaderAfterHunkJump(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 2, 3, 1)}, Session{}, nil)
	for i := 0; i < 5; i++ {
		m.jumpToHunk(1)
		assertNotOnHeader(t, m, "jumpToHunk forward")
	}
	for i := 0; i < 5; i++ {
		m.jumpToHunk(-1)
		assertNotOnHeader(t, m, "jumpToHunk backward")
	}
}

func TestCursorNeverOnHeaderAfterFileSwitch(t *testing.T) {
	m := newModel("/repo", []FileDiff{
		fileDiffWithHunks("a.go", 3),
		fileDiffWithHunks("b.go", 4),
	}, Session{}, nil)

	mm, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m2 := mm.(model)
	assertNotOnHeader(t, m2, "tab (next file)")

	mm, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m3 := mm.(model)
	assertNotOnHeader(t, m3, "shift+tab (prev file)")
}

func TestCursorNeverOnHeaderAfterSetDiffFiles(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 10)}, Session{}, nil)
	m.lineIndex = 8 // deep into the file

	// Shrink the file drastically on refresh.
	m.setDiffFiles([]FileDiff{fileDiffWithHunks("a.go", 2)})
	assertNotOnHeader(t, m, "setDiffFiles after shrinking")
}
