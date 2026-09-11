package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// diffLayout is buildDiffLinesDetailed's cheap pass: how many physical
// lines each row contributes, without paying for syntax highlighting.
type diffLayout struct {
	key     string
	heights []int
	// offsets has len(rows)+1 entries; offsets[i] is row i's first
	// physical line, offsets[len(rows)] is the file's total physical line
	// count.
	offsets []int
}

// diffLinesCache is buildDiffLinesDetailed's memoized full output.
type diffLinesCache struct {
	key        string
	lines      []string
	cursorLine int
	rowFor     []int
	mainLine   []bool
}

// \x00 between fields (here and in diffLayoutKey) guards against two
// different states hashing the same — without it, editingCommentID="ab",
// editingReplyID="c" would collide with editingCommentID="a",
// editingReplyID="bc".
func diffLinesCacheKey(fr fileRows, byRow map[int][]rowComment, width int, wrapLines, showLineNumbers bool, mode mode, lineIndex int, editingCommentID, editingReplyID, replyingToCommentID, input string, commentExpanded map[string]bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s\x00%s\x00%s\x00%s\x00",
		diffLayoutKey(fr, byRow, width, wrapLines, showLineNumbers, commentExpanded),
		mode, lineIndex, editingCommentID, editingReplyID, replyingToCommentID, input)
	return hex.EncodeToString(h.Sum(nil))
}

// diffRenderMarginRows: rows within this many of the cursor get the
// expensive treatment; any real terminal viewport is well under this.
var diffRenderMarginRows = 300

// currentDiffLayout computes the "at rest" layout — every row as it looks
// outside of active comment editing. Editing only ever targets the comment
// on the cursor's own row (see commentActionForCurrentLine), so mode/
// lineIndex/editing state isn't part of the key — that would invalidate
// every row's height on every keystroke, not just the cursor row's.
func (m model) currentDiffLayout(fr fileRows, byRow map[int][]rowComment, width int) *diffLayout {
	key := diffLayoutKey(fr, byRow, width, m.wrapLines, m.showLineNumbers, m.commentExpanded)
	if m.layoutCache != nil && m.layoutCache.key == key {
		return m.layoutCache
	}

	rows := fr.rows
	heights := make([]int, len(rows))
	offsets := make([]int, len(rows)+1)
	for i, row := range rows {
		heights[i] = rowPhysicalHeight(m, fr, row, byRow[i], width)
		offsets[i+1] = offsets[i] + heights[i]
	}

	layout := &diffLayout{key: key, heights: heights, offsets: offsets}
	if m.layoutCache != nil {
		*m.layoutCache = *layout
	}
	return layout
}

func diffLayoutKey(fr fileRows, byRow map[int][]rowComment, width int, wrapLines, showLineNumbers bool, commentExpanded map[string]bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%d\x00%v\x00%v\x00",
		fr.file.Path, fr.hash, width, wrapLines, showLineNumbers)

	rowIdxs := make([]int, 0, len(byRow))
	for idx := range byRow {
		rowIdxs = append(rowIdxs, idx)
	}
	sort.Ints(rowIdxs)
	for _, idx := range rowIdxs {
		for _, rc := range byRow[idx] {
			fmt.Fprintf(h, "%d\x00%s\x00%s\x00%v\x00%v\x00", idx, rc.ID, rc.Body, rc.Resolved, commentExpanded[rc.ID])
			for _, r := range rc.Replies {
				fmt.Fprintf(h, "%s\x00%s\x00", r.ID, r.Body)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// rowPhysicalHeight mirrors buildDiffLinesDetailed's appendText loop, but
// measures plain (unstyled) text — wrapping only cares about visible
// width, which styling never changes, so this gives an identical count
// without syntax-highlighting's cost.
func rowPhysicalHeight(m model, fr fileRows, row diffRow, comments []rowComment, width int) int {
	if row.kind == rowHunkHeader {
		return wrapLineCount("@@ "+row.hunkHeader, width, m.wrapLines, 0)
	}

	total := wrapLineCount(plainLineText(row.line, m.showLineNumbers, fr.numWidth), width, m.wrapLines, 0)

	for _, c := range comments {
		if !m.isCommentExpanded(c.Comment) {
			total += multilineWrapCount(renderCollapsedComment(c.Comment, c.Stale), width, m.wrapLines)
			continue
		}
		total += multilineWrapCount(renderComment(c.Comment, c.Stale), width, m.wrapLines)
		for _, r := range c.Replies {
			total += multilineWrapCount(renderReply(r, c.Resolved, false), width, m.wrapLines)
		}
	}
	return total
}

// plainLineText is renderLine's text without styling — same width, none
// of the chroma/lipgloss cost.
func plainLineText(l Line, showNumbers bool, numWidth int) string {
	prefix := " "
	switch l.Kind {
	case LineAdded:
		prefix = "+"
	case LineRemoved:
		prefix = "-"
	}
	content := prefix + strings.ReplaceAll(l.Content, "\t", "    ")
	if !showNumbers {
		return content
	}
	return fmt.Sprintf("%*s %*s  ", numWidth, lineNumStr(l.OldLine), numWidth, lineNumStr(l.NewLine)) + content
}

// effectiveWrapWidth is appendText's own width-selection rule, factored out
// so the layout pass can't drift from what actually gets rendered.
func effectiveWrapWidth(width, indent int) int {
	if indent == commentIndentWidth && width > maxCommentWrapWidth {
		return maxCommentWrapWidth
	}
	return width
}

func wrapLineCount(text string, width int, wrapLines bool, indent int) int {
	if !wrapLines {
		return 1
	}
	return len(wrapLineIndented(text, effectiveWrapWidth(width, indent), indent, ""))
}

func multilineWrapCount(text string, width int, wrapLines bool) int {
	n := 0
	for _, l := range strings.Split(text, "\n") {
		n += wrapLineCount(l, width, wrapLines, commentIndentWidth)
	}
	return n
}

// currentSplitDiffLayout is currentDiffLayout's split-view counterpart —
// a content row's height is the taller of its two sides.
func (m model) currentSplitDiffLayout(fr fileRows, byRow map[int][]rowComment, width int) *diffLayout {
	key := diffLayoutKey(fr, byRow, width, m.wrapLines, m.showLineNumbers, m.commentExpanded)
	if m.splitLayoutCache != nil && m.splitLayoutCache.key == key {
		return m.splitLayoutCache
	}

	leftW, rightW := splitColumnWidths(width)
	rows := fr.splitRows
	heights := make([]int, len(rows))
	offsets := make([]int, len(rows)+1)
	for i, row := range rows {
		heights[i] = rowPhysicalHeightSplit(m, fr, row, byRow[i], width, leftW, rightW)
		offsets[i+1] = offsets[i] + heights[i]
	}

	layout := &diffLayout{key: key, heights: heights, offsets: offsets}
	if m.splitLayoutCache != nil {
		*m.splitLayoutCache = *layout
	}
	return layout
}

func rowPhysicalHeightSplit(m model, fr fileRows, row pairedRow, comments []rowComment, width, leftW, rightW int) int {
	if row.kind == rowHunkHeader {
		return wrapLineCount("@@ "+row.hunkHeader, width, m.wrapLines, 0)
	}

	total := max(splitSidePlainHeight(row.left, m.showLineNumbers, fr.numWidth, m.wrapLines, leftW),
		splitSidePlainHeight(row.right, m.showLineNumbers, fr.numWidth, m.wrapLines, rightW))

	for _, c := range comments {
		if !m.isCommentExpanded(c.Comment) {
			total += multilineWrapCount(renderCollapsedComment(c.Comment, c.Stale), width, m.wrapLines)
			continue
		}
		total += multilineWrapCount(renderComment(c.Comment, c.Stale), width, m.wrapLines)
		for _, r := range c.Replies {
			total += multilineWrapCount(renderReply(r, c.Resolved, false), width, m.wrapLines)
		}
	}
	return total
}

// splitSidePlainHeight is splitSidePhysicalLines' line-count-only
// counterpart, measuring unstyled text.
func splitSidePlainHeight(l *Line, showNumbers bool, numWidth int, wrapLines bool, width int) int {
	if l == nil || !wrapLines {
		return 1
	}
	return len(wrapLine(plainLineText(*l, showNumbers, numWidth), width))
}
