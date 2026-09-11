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
	rows, _ := flattenFileSplitWithBounds(fd)
	return rows
}

// flattenFileSplitWithBounds is flattenFileSplit plus each hunk's row
// range within the result — hunkStart[i] is hunk i's header row index,
// hunkStart[len(fd.Hunks)] is len(rows) — so a lazy per-hunk operation
// (see ensureHunkTokenized) knows which rows to touch.
func flattenFileSplitWithBounds(fd FileDiff) (rows []pairedRow, hunkStart []int) {
	hunkStart = make([]int, 0, len(fd.Hunks)+1)
	for _, h := range fd.Hunks {
		hunkStart = append(hunkStart, len(rows))
		rows = append(rows, pairedRow{kind: rowHunkHeader, hunkHeader: h.Header})
		rows = append(rows, pairHunkLines(h.Lines)...)
	}
	hunkStart = append(hunkStart, len(rows))
	return rows, hunkStart
}

// pairHunkLines pairs one hunk's lines: LineContext rows are always 1:1
// (they can't differ in count between sides, so both left and right point
// at the same Line), and each maximal run of non-context lines is zipped
// via zipChangeBlock. left/right point directly into lines' backing array
// (not a copy) — see ensureHunkTokenized, which relies on this to make a
// hunk's lazily-computed syntax tokens visible through split rows without
// a separate backfill step.
func pairHunkLines(lines []Line) []pairedRow {
	var rows []pairedRow
	i := 0
	for i < len(lines) {
		if lines[i].Kind == LineContext {
			rows = append(rows, pairedRow{kind: rowLine, left: &lines[i], right: &lines[i]})
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
// run's remainder becomes left-only or right-only rows. Pointers here also
// point into block's (and so the hunk's) backing array — see pairHunkLines.
func zipChangeBlock(block []Line) []pairedRow {
	var removed, added []*Line
	for i := range block {
		switch block[i].Kind {
		case LineRemoved:
			removed = append(removed, &block[i])
		case LineAdded:
			added = append(added, &block[i])
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
			row.left = removed[i]
		}
		if i < len(added) {
			row.right = added[i]
		}
		rows[i] = row
	}
	return rows
}
