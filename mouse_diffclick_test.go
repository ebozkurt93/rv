package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Coordinates verified against a real rendered frame for this exact
// configuration: header=3 lines, diff panel top border at y=3, content
// rows starting y=4 (hunk header), y=5 = line 1, y=6 = line 2, etc.

func TestClickDiffLineFocusesThatRow(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 5)}, Session{}, nil)
	m.width, m.height = 100, 24

	mm, _ := m.Update(tea.MouseClickMsg{X: 60, Y: 7, Button: tea.MouseLeft})
	m2 := mm.(model)

	line, ok := m2.currentLine()
	if !ok || line.NewLine == nil || *line.NewLine != 3 {
		t.Fatalf("expected clicking y=7 to focus line 3, got line=%+v ok=%v", line, ok)
	}
}

func TestClickDiffHunkHeaderSnapsToNearestContentRow(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithHunks("a.go", 3, 3)}, Session{}, nil)
	m.width, m.height = 100, 24
	m.lineIndex = 4 // deep into the 2nd hunk, so the click is a real move

	// y=4 is the (first) hunk header's screen row.
	mm, _ := m.Update(tea.MouseClickMsg{X: 60, Y: 4, Button: tea.MouseLeft})
	m2 := mm.(model)

	assertNotOnHeader(t, m2, "clicking a hunk header")
	if m2.lineIndex != 1 {
		t.Fatalf("expected snapping forward to row 1 (the header's first content line), got %d", m2.lineIndex)
	}
}

func TestClickDiffLineWithWrapFocusesCorrectRowRegardlessOfWhichSubLine(t *testing.T) {
	withTempHome(t) // newModel loads persisted UI prefs (wrap included) — sandbox it
	long := "this is a deliberately long line meant to wrap across more than one physical terminal row when rendered"
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{
		Header: "h",
		Lines: []Line{
			{Kind: LineContext, Content: "short one", OldLine: intp(1), NewLine: intp(1)},
			{Kind: LineContext, Content: long, OldLine: intp(2), NewLine: intp(2)},
			{Kind: LineContext, Content: "short two", OldLine: intp(3), NewLine: intp(3)},
		},
	}}}
	m := newModel("/repo", []FileDiff{fd}, Session{}, nil)
	m.width, m.height = 60, 24
	mm, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"}) // enable wrap
	m = mm.(model)

	lines, _, rowFor := m.buildDiffLines(m.width - m.diffPaneSidebarWidth() - borderOverheadW)
	if len(lines) <= 4 {
		t.Fatalf("expected the long line to wrap into multiple rendered lines, got %d total lines", len(lines))
	}

	// Find a rendered line belonging to row 2 (the long line) that ISN'T
	// its first physical line — proof the click resolves through wrapping,
	// not just luckily landing on row 2's first sub-line.
	subLineIdx := -1
	for i := 1; i < len(rowFor); i++ {
		if rowFor[i] == 2 && rowFor[i-1] == 2 {
			subLineIdx = i
			break
		}
	}
	if subLineIdx < 0 {
		t.Fatal("expected at least 2 consecutive rendered lines for row 2 (proof it actually wrapped)")
	}

	y := m.headerHeight() + 1 + subLineIdx
	// X must land in the diff pane, not the sidebar.
	mm, _ = m.Update(tea.MouseClickMsg{X: m.sidebarWidth() + 2, Y: y, Button: tea.MouseLeft})
	m2 := mm.(model)

	line, ok := m2.currentLine()
	if !ok || line.NewLine == nil || *line.NewLine != 2 {
		t.Fatalf("expected clicking the wrapped line's 2nd physical row to still focus new-line 2, got %+v ok=%v", line, ok)
	}
}
