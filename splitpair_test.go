package main

import "testing"

func TestPairHunkLinesEqualCountsZipOneToOne(t *testing.T) {
	lines := []Line{
		{Kind: LineRemoved, Content: "old1", OldLine: intp(1)},
		{Kind: LineRemoved, Content: "old2", OldLine: intp(2)},
		{Kind: LineAdded, Content: "new1", NewLine: intp(1)},
		{Kind: LineAdded, Content: "new2", NewLine: intp(2)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 2 {
		t.Fatalf("expected 2 paired rows, got %d", len(rows))
	}
	for i, want := range []struct{ left, right string }{{"old1", "new1"}, {"old2", "new2"}} {
		r := rows[i]
		if r.left == nil || r.left.Content != want.left {
			t.Fatalf("row %d: expected left=%q, got %+v", i, want.left, r.left)
		}
		if r.right == nil || r.right.Content != want.right {
			t.Fatalf("row %d: expected right=%q, got %+v", i, want.right, r.right)
		}
	}
}

func TestPairHunkLinesMoreRemovedThanAdded(t *testing.T) {
	lines := []Line{
		{Kind: LineRemoved, Content: "old1", OldLine: intp(1)},
		{Kind: LineRemoved, Content: "old2", OldLine: intp(2)},
		{Kind: LineRemoved, Content: "old3", OldLine: intp(3)},
		{Kind: LineAdded, Content: "new1", NewLine: intp(1)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 3 {
		t.Fatalf("expected 3 paired rows, got %d", len(rows))
	}
	if rows[0].left.Content != "old1" || rows[0].right.Content != "new1" {
		t.Fatalf("row 0: expected old1/new1 pair, got %+v", rows[0])
	}
	for i := 1; i < 3; i++ {
		if rows[i].right != nil {
			t.Fatalf("row %d: expected right-only unmatched, got right=%+v", i, rows[i].right)
		}
		if rows[i].left == nil {
			t.Fatalf("row %d: expected left populated, got nil", i)
		}
	}
	if rows[1].left.Content != "old2" || rows[2].left.Content != "old3" {
		t.Fatalf("expected unmatched remainder in order, got %+v / %+v", rows[1].left, rows[2].left)
	}
}

func TestPairHunkLinesMoreAddedThanRemoved(t *testing.T) {
	lines := []Line{
		{Kind: LineRemoved, Content: "old1", OldLine: intp(1)},
		{Kind: LineAdded, Content: "new1", NewLine: intp(1)},
		{Kind: LineAdded, Content: "new2", NewLine: intp(2)},
		{Kind: LineAdded, Content: "new3", NewLine: intp(3)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 3 {
		t.Fatalf("expected 3 paired rows, got %d", len(rows))
	}
	if rows[0].left.Content != "old1" || rows[0].right.Content != "new1" {
		t.Fatalf("row 0: expected old1/new1 pair, got %+v", rows[0])
	}
	for i := 1; i < 3; i++ {
		if rows[i].left != nil {
			t.Fatalf("row %d: expected left-only unmatched, got left=%+v", i, rows[i].left)
		}
		if rows[i].right == nil {
			t.Fatalf("row %d: expected right populated, got nil", i)
		}
	}
	if rows[1].right.Content != "new2" || rows[2].right.Content != "new3" {
		t.Fatalf("expected unmatched remainder in order, got %+v / %+v", rows[1].right, rows[2].right)
	}
}

func TestPairHunkLinesContextLinesAreOneToOneOnBothSides(t *testing.T) {
	lines := []Line{
		{Kind: LineContext, Content: "ctx1", OldLine: intp(1), NewLine: intp(1)},
		{Kind: LineRemoved, Content: "old1", OldLine: intp(2)},
		{Kind: LineAdded, Content: "new1", NewLine: intp(2)},
		{Kind: LineContext, Content: "ctx2", OldLine: intp(3), NewLine: intp(3)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 3 {
		t.Fatalf("expected 3 paired rows, got %d", len(rows))
	}
	if rows[0].left != rows[0].right {
		t.Fatalf("expected context row's left/right to point at the same Line, got %p vs %p", rows[0].left, rows[0].right)
	}
	if rows[0].left.Content != "ctx1" {
		t.Fatalf("expected first row to be ctx1, got %+v", rows[0].left)
	}
	if rows[2].left != rows[2].right || rows[2].left.Content != "ctx2" {
		t.Fatalf("expected last row to be ctx2 on both sides, got %+v", rows[2])
	}
	if rows[1].left.Content != "old1" || rows[1].right.Content != "new1" {
		t.Fatalf("expected middle row to pair old1/new1, got %+v", rows[1])
	}
}

func TestPairHunkLinesOrderReversedBlockStillPairs(t *testing.T) {
	// Added lines appearing before removed lines within the same
	// contiguous change block — order between the two kinds shouldn't
	// matter, only each kind's own relative order.
	lines := []Line{
		{Kind: LineAdded, Content: "new1", NewLine: intp(1)},
		{Kind: LineRemoved, Content: "old1", OldLine: intp(1)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 1 {
		t.Fatalf("expected 1 paired row, got %d", len(rows))
	}
	if rows[0].left.Content != "old1" || rows[0].right.Content != "new1" {
		t.Fatalf("expected old1/new1 pair regardless of raw order, got %+v", rows[0])
	}
}

func TestPairHunkLinesPureAdditionOnly(t *testing.T) {
	lines := []Line{
		{Kind: LineAdded, Content: "new1", NewLine: intp(1)},
		{Kind: LineAdded, Content: "new2", NewLine: intp(2)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 2 {
		t.Fatalf("expected 2 paired rows, got %d", len(rows))
	}
	for i, want := range []string{"new1", "new2"} {
		if rows[i].left != nil {
			t.Fatalf("row %d: expected no left side, got %+v", i, rows[i].left)
		}
		if rows[i].right == nil || rows[i].right.Content != want {
			t.Fatalf("row %d: expected right=%q, got %+v", i, want, rows[i].right)
		}
	}
}

func TestPairHunkLinesPureRemovalOnly(t *testing.T) {
	lines := []Line{
		{Kind: LineRemoved, Content: "old1", OldLine: intp(1)},
		{Kind: LineRemoved, Content: "old2", OldLine: intp(2)},
	}
	rows := pairHunkLines(lines)
	if len(rows) != 2 {
		t.Fatalf("expected 2 paired rows, got %d", len(rows))
	}
	for i, want := range []string{"old1", "old2"} {
		if rows[i].right != nil {
			t.Fatalf("row %d: expected no right side, got %+v", i, rows[i].right)
		}
		if rows[i].left == nil || rows[i].left.Content != want {
			t.Fatalf("row %d: expected left=%q, got %+v", i, want, rows[i].left)
		}
	}
}

func TestFlattenFileSplitIncludesHunkHeaders(t *testing.T) {
	fd := FileDiff{
		Path:   "a.go",
		Status: FileModified,
		Hunks: []Hunk{
			{Header: "h1", Lines: []Line{
				{Kind: LineContext, Content: "ctx", OldLine: intp(1), NewLine: intp(1)},
			}},
			{Header: "h2", Lines: []Line{
				{Kind: LineRemoved, Content: "old1", OldLine: intp(5)},
				{Kind: LineAdded, Content: "new1", NewLine: intp(5)},
			}},
		},
	}
	rows := flattenFileSplit(fd)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (2 headers + 2 content rows), got %d", len(rows))
	}
	if !rows[0].isHeader() || rows[0].hunkHeader != "h1" {
		t.Fatalf("expected row 0 to be header h1, got %+v", rows[0])
	}
	if rows[1].isHeader() {
		t.Fatalf("expected row 1 to be a content row, got header")
	}
	if !rows[2].isHeader() || rows[2].hunkHeader != "h2" {
		t.Fatalf("expected row 2 to be header h2, got %+v", rows[2])
	}
	if rows[3].isHeader() {
		t.Fatalf("expected row 3 to be a content row, got header")
	}
}

func TestPairedRowSatisfiesContentRowHelpers(t *testing.T) {
	rows := []pairedRow{
		{kind: rowHunkHeader, hunkHeader: "h"},
		{kind: rowLine, left: &Line{Content: "a"}},
		{kind: rowLine, right: &Line{Content: "b"}},
	}
	if got := firstContentRow(rows); got != 1 {
		t.Fatalf("expected firstContentRow=1, got %d", got)
	}
	if got := lastContentRow(rows); got != 2 {
		t.Fatalf("expected lastContentRow=2, got %d", got)
	}
	if got := nearestContentRow(rows, 0); got != 1 {
		t.Fatalf("expected nearestContentRow(0)=1, got %d", got)
	}
}
