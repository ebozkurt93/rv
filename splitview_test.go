package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// modifiedPairFileDiff builds a file with one hunk: a leading context line,
// a genuine 1:1 modified pair (one removed, one added), and a trailing
// context line — enough to exercise pairing, cursor navigation, and
// comment anchoring across both view modes.
func modifiedPairFileDiff(path string) FileDiff {
	l1, l3 := 1, 3
	oldLine, newLine := 2, 2
	return FileDiff{Path: path, Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineContext, Content: "before", OldLine: &l1, NewLine: &l1},
		{Kind: LineRemoved, Content: "old body", OldLine: &oldLine},
		{Kind: LineAdded, Content: "new body", NewLine: &newLine},
		{Kind: LineContext, Content: "after", OldLine: &l3, NewLine: &l3},
	}}}}
}

func TestToggleSplitViewFlipsSplitView(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{modifiedPairFileDiff("a.go")}, Session{}, nil)
	if m.splitView {
		t.Fatal("expected unified view by default")
	}

	mm, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m2 := mm.(model)
	if !m2.splitView {
		t.Fatal("expected split view after s")
	}

	mm, _ = m2.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m3 := mm.(model)
	if m3.splitView {
		t.Fatal("expected unified view again after second s")
	}
}

func TestToggleSplitViewPersistsAcrossNewModel(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", nil, Session{}, nil)
	if m.splitView {
		t.Fatal("expected split view off by default")
	}

	mm, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m2 := mm.(model)
	m2.persistUIPrefs()

	m3 := newModel("/repo", nil, Session{}, nil)
	if !m3.splitView {
		t.Fatal("expected split view to persist as on across a fresh newModel")
	}
}

func TestSplitViewCursorMovementStaysOnContentRows(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{modifiedPairFileDiff("a.go")}, Session{}, nil)
	m.splitView = true
	m.lineIndex = firstContentRow(m.currentSplitRows())

	// rows[0] is the hunk header; rows[1]="before", rows[2]=the modified
	// pair, rows[3]="after".
	rows := m.currentSplitRows()
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (1 header + before/pair/after), got %d", len(rows))
	}
	if m.lineIndex != 1 {
		t.Fatalf("expected firstContentRow to skip the header and land on row 1, got %d", m.lineIndex)
	}

	m.moveCursor(1)
	if m.lineIndex != 2 {
		t.Fatalf("expected cursor on the modified pair (row 2), got %d", m.lineIndex)
	}
	if rows[m.lineIndex].left == nil || rows[m.lineIndex].left.Content != "old body" {
		t.Fatalf("expected row 2's left side to be the removed line, got %+v", rows[m.lineIndex].left)
	}
	if rows[m.lineIndex].right == nil || rows[m.lineIndex].right.Content != "new body" {
		t.Fatalf("expected row 2's right side to be the added line, got %+v", rows[m.lineIndex].right)
	}

	m.moveCursor(1)
	if m.lineIndex != 3 {
		t.Fatalf("expected cursor on the trailing context row (row 3), got %d", m.lineIndex)
	}

	// Boundary: moving past the end is a no-op, not an out-of-range index.
	m.moveCursor(5)
	if m.lineIndex != 3 {
		t.Fatalf("expected cursor to stay clamped at row 3, got %d", m.lineIndex)
	}
}

func TestCurrentLinePrefersRightSideInSplitView(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{modifiedPairFileDiff("a.go")}, Session{}, nil)
	m.splitView = true
	m.lineIndex = 2 // the modified pair

	line, ok := m.currentLine()
	if !ok {
		t.Fatal("expected currentLine to find a commentable row")
	}
	if line.Content != "new body" {
		t.Fatalf("expected currentLine to prefer the new (right) side, got %q", line.Content)
	}
}

func TestSplitViewCommentAnchorsAndSurvivesViewToggle(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{modifiedPairFileDiff("a.go")}, Session{}, nil)
	m.splitView = true
	m.lineIndex = 2 // the modified pair, right side is "new body" / NewLine 2

	m.addCommentUnderCursor() // no-op: input is empty
	m.input = "looks good"
	m.addCommentUnderCursor()

	if len(m.session.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(m.session.Comments))
	}
	c := m.session.Comments[0]
	if c.NewLine == nil || *c.NewLine != 2 {
		t.Fatalf("expected comment anchored to new-side line 2, got %+v", c)
	}

	// Still findable via commentsForCurrentLine while still in split view.
	if got := m.commentsForCurrentLine(); len(got) != 1 {
		t.Fatalf("expected 1 comment on the current split row, got %d", len(got))
	}

	// Toggle to unified view — the same comment (anchored by line number +
	// content, see commentAnchorRow) must still resolve to the row holding
	// the added line, without needing separate re-anchoring logic per view.
	m.splitView = false
	m.lineIndex = nearestContentRow(m.currentRows(), 0)
	fi, ri, ok := m.locateComment(c)
	if !ok {
		t.Fatal("expected locateComment to find the comment in unified view too")
	}
	if fi != 0 {
		t.Fatalf("expected fileIdx 0, got %d", fi)
	}
	unifiedRows := m.currentRows()
	if ri < 0 || ri >= len(unifiedRows) || unifiedRows[ri].kind != rowLine || unifiedRows[ri].line.Content != "new body" {
		t.Fatalf("expected the located row to be the added line, got %+v", unifiedRows[ri])
	}
}

func TestSplitViewMouseClickSelectsPairedRow(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{modifiedPairFileDiff("a.go")}, Session{}, nil)
	m.splitView = true
	m.width, m.height = 100, 24

	// +1 for the diff panel's own top border row. Inside the panel: the
	// hunk header is the first rendered line, then "before", then the
	// modified pair — two rows below the header.
	y := m.headerHeight() + 1 + 2
	m.clickDiffRow(y)

	if m.lineIndex != 2 {
		t.Fatalf("expected click to land on the modified pair (row 2), got lineIndex=%d", m.lineIndex)
	}
}

func TestSplitSidePhysicalLinesWrapsAndPadsToMatch(t *testing.T) {
	lexer := pickLexer("a.go")
	newLine := 1
	// A long added line whose right side needs 2 physical lines to wrap
	// at this width; the left side (nil — no content, an unmatched
	// added-only row) must come back padded to match that count.
	added := &Line{Kind: LineAdded, Content: "this is a fairly long line of content", NewLine: &newLine}

	rightLines := splitSidePhysicalLines(lexer, added, false, 1, false, true, 20)
	if len(rightLines) < 2 {
		t.Fatalf("expected the long line to wrap to at least 2 physical lines at width 20, got %d", len(rightLines))
	}

	leftLines := splitSidePhysicalLines(lexer, nil, false, 1, false, true, 20)
	if len(leftLines) != 1 {
		t.Fatalf("expected a nil side to always report exactly 1 physical line, got %d", len(leftLines))
	}

	// buildSplitDiffLines pads the shorter side with splitPadLine for each
	// extra row the other side's wrap produced.
	pad := splitPadLine(nil, false, 20)
	if got := ansi.StringWidth(pad); got != 20 {
		t.Fatalf("expected the pad line to be exactly width 20, got %d (%q)", got, pad)
	}
}
