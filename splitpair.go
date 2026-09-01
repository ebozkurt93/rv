package main

// pairedRow is one row of the split (side-by-side) diff view: either a hunk
// header (shared across both columns) or a paired line. left/right point at
// the same underlying Line for a context row (present on both sides), at
// their own respective Line for a genuine modified pair, or are nil when
// this row has no content on that side (an unmatched removed- or added-only
// row within a run whose counts differ).
type pairedRow struct {
	kind       rowKind
	hunkHeader string
	left       *Line
	right      *Line
}

func (r pairedRow) isHeader() bool { return r.kind == rowHunkHeader }

// flattenFileSplit is flattenFile's split-view counterpart: instead of one
// row per Line, it pairs up a hunk's removed/added runs positionally (by
// index within each run, not by content similarity — see
// research/split-view-design.md point 1 for why that's sufficient for a
// first pass) so a modified line shows old and new side by side, the way
// GitHub's split diff view and diff-so-fancy do structurally.
func flattenFileSplit(fd FileDiff) []pairedRow {
	var rows []pairedRow
	for _, h := range fd.Hunks {
		rows = append(rows, pairedRow{kind: rowHunkHeader, hunkHeader: h.Header})
		rows = append(rows, pairHunkLines(h.Lines)...)
	}
	return rows
}

// pairHunkLines pairs one hunk's lines: LineContext rows are always 1:1
// (they can't differ in count between sides, so both left and right point
// at the same Line), and each maximal run of non-context lines is zipped
// via zipChangeBlock.
func pairHunkLines(lines []Line) []pairedRow {
	var rows []pairedRow
	i := 0
	for i < len(lines) {
		if lines[i].Kind == LineContext {
			l := lines[i]
			rows = append(rows, pairedRow{kind: rowLine, left: &l, right: &l})
			i++
			continue
		}

		start := i
		for i < len(lines) && lines[i].Kind != LineContext {
			i++
		}
		rows = append(rows, zipChangeBlock(lines[start:i])...)
	}
	return rows
}

// zipChangeBlock pairs one maximal run of non-context lines: its Removed
// and Added lines are separated out (preserving each kind's own relative
// order — the run's raw interleaving of the two kinds doesn't matter, see
// research/split-view-design.md point 1), then zipped index-by-index.
// min(len(removed), len(added)) rows get both sides populated; the longer
// run's remainder becomes left-only or right-only rows.
func zipChangeBlock(block []Line) []pairedRow {
	var removed, added []Line
	for _, l := range block {
		switch l.Kind {
		case LineRemoved:
			removed = append(removed, l)
		case LineAdded:
			added = append(added, l)
		}
	}

	n := len(removed)
	if len(added) > n {
		n = len(added)
	}
	rows := make([]pairedRow, n)
	for i := 0; i < n; i++ {
		row := pairedRow{kind: rowLine}
		if i < len(removed) {
			l := removed[i]
			row.left = &l
		}
		if i < len(added) {
			l := added[i]
			row.right = &l
		}
		rows[i] = row
	}
	return rows
}
