package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// fileDiffWithVariedLines builds n lines, some long enough to force
// wrapping at a narrow width.
func fileDiffWithVariedLines(path string, n int) FileDiff {
	var lines []Line
	for i := 1; i <= n; i++ {
		ln := i
		content := fmt.Sprintf("line %d", i)
		if i%7 == 0 {
			content = strings.Repeat(fmt.Sprintf("word%d ", i), 20) // forces wrap at narrow widths
		}
		lines = append(lines, Line{Kind: LineContext, Content: content, OldLine: &ln, NewLine: &ln})
	}
	return FileDiff{Path: path, Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: lines}}}
}

// TestWindowedRenderMatchesFullRender checks a small diffRenderMarginRows
// (only rows near the cursor rendered) against an effectively unwindowed
// one (huge margin) — same cursorLine, total length, and visible content.
func TestWindowedRenderMatchesFullRender(t *testing.T) {
	origMargin := diffRenderMarginRows
	defer func() { diffRenderMarginRows = origMargin }()

	fd := fileDiffWithVariedLines("a.go", 500)

	for _, wrapLines := range []bool{false, true} {
		for _, showNumbers := range []bool{false, true} {
			for _, cursorRow := range []int{0, 250, 499} {
				name := fmt.Sprintf("wrap=%v numbers=%v cursor=%d", wrapLines, showNumbers, cursorRow)
				t.Run(name, func(t *testing.T) {
					m := newModel("/repo", []FileDiff{fd}, Session{}, nil)
					m.wrapLines = wrapLines
					m.showLineNumbers = showNumbers
					m.lineIndex = cursorRow
					m.width, m.height = 100, 40
					width := 60

					diffRenderMarginRows = 5
					m.layoutCache = map[string]diffLayout{}
					m.diffLinesCache = map[string]diffLinesCache{}
					wLines, wCursor, wRowFor, _ := m.buildDiffLinesDetailed(width)

					diffRenderMarginRows = 100000
					m.layoutCache = map[string]diffLayout{}
					m.diffLinesCache = map[string]diffLinesCache{}
					fLines, fCursor, fRowFor, _ := m.buildDiffLinesDetailed(width)

					if len(wLines) != len(fLines) {
						t.Fatalf("total physical lines mismatch: windowed=%d full=%d", len(wLines), len(fLines))
					}
					if wCursor != fCursor {
						t.Fatalf("cursorLine mismatch: windowed=%d full=%d", wCursor, fCursor)
					}

					lo, hi := wCursor-5, wCursor+5
					if lo < 0 {
						lo = 0
					}
					if hi > len(wLines) {
						hi = len(wLines)
					}
					for i := lo; i < hi; i++ {
						if wLines[i] != fLines[i] {
							t.Errorf("line %d content mismatch\n  windowed: %q\n  full:     %q", i, wLines[i], fLines[i])
						}
						if wRowFor[i] != fRowFor[i] {
							t.Errorf("line %d rowFor mismatch: windowed=%d full=%d", i, wRowFor[i], fRowFor[i])
						}
					}
				})
			}
		}
	}
}

// TestWindowedRenderAccountsForOffscreenComments checks a comment outside
// the rendered window still contributes its correct height to the total.
func TestWindowedRenderAccountsForOffscreenComments(t *testing.T) {
	origMargin := diffRenderMarginRows
	defer func() { diffRenderMarginRows = origMargin }()

	fd := fileDiffWithVariedLines("a.go", 500)
	n := 10
	session := Session{Comments: []Comment{
		{ID: "c1", File: "a.go", NewLine: &n, LineContent: "line 10", Body: "a fairly long comment body that will wrap across several lines at a narrow width"},
	}}

	m := newModel("/repo", []FileDiff{fd}, session, nil)
	m.wrapLines = true
	m.lineIndex = 250
	width := 60

	diffRenderMarginRows = 5
	m.layoutCache = map[string]diffLayout{}
	m.diffLinesCache = map[string]diffLinesCache{}
	wLines, wCursor, _, _ := m.buildDiffLinesDetailed(width)

	diffRenderMarginRows = 100000
	m.layoutCache = map[string]diffLayout{}
	m.diffLinesCache = map[string]diffLinesCache{}
	fLines, fCursor, _, _ := m.buildDiffLinesDetailed(width)

	if len(wLines) != len(fLines) {
		t.Fatalf("total physical lines mismatch: windowed=%d full=%d", len(wLines), len(fLines))
	}
	if wCursor != fCursor {
		t.Fatalf("cursorLine mismatch: windowed=%d full=%d", wCursor, fCursor)
	}
}

// TestWindowedRenderDuringCommentEditing checks windowed and full renders
// still agree while the comment editor is open on the cursor row (see
// currentDiffLayout's doc comment on why editing state isn't cache-keyed).
func TestWindowedRenderDuringCommentEditing(t *testing.T) {
	origMargin := diffRenderMarginRows
	defer func() { diffRenderMarginRows = origMargin }()

	fd := fileDiffWithVariedLines("a.go", 500)
	m := newModel("/repo", []FileDiff{fd}, Session{}, nil)
	m.wrapLines = true
	m.lineIndex = 250
	m.mode = modeComment
	m.input = "typing a new comment here\nwith a second line"
	width := 60

	diffRenderMarginRows = 5
	m.layoutCache = map[string]diffLayout{}
	m.diffLinesCache = map[string]diffLinesCache{}
	wLines, wCursor, _, _ := m.buildDiffLinesDetailed(width)

	diffRenderMarginRows = 100000
	m.layoutCache = map[string]diffLayout{}
	m.diffLinesCache = map[string]diffLinesCache{}
	fLines, fCursor, _, _ := m.buildDiffLinesDetailed(width)

	if len(wLines) != len(fLines) {
		t.Fatalf("total physical lines mismatch: windowed=%d full=%d", len(wLines), len(fLines))
	}
	if wCursor != fCursor {
		t.Fatalf("cursorLine mismatch: windowed=%d full=%d", wCursor, fCursor)
	}
	for i := wCursor; i < len(wLines) && i < wCursor+10; i++ {
		if wLines[i] != fLines[i] {
			t.Errorf("line %d content mismatch\n  windowed: %q\n  full:     %q", i, wLines[i], fLines[i])
		}
	}
}

// fileDiffWithVariedModifiedLines builds n removed+added line pairs, some
// long enough to force wrapping, so both sides of a pairedRow differ.
func fileDiffWithVariedModifiedLines(path string, n int) FileDiff {
	var lines []Line
	for i := 1; i <= n; i++ {
		oldN, newN := i, i
		oldContent := fmt.Sprintf("old line %d", i)
		newContent := fmt.Sprintf("new line %d", i)
		if i%7 == 0 {
			oldContent = strings.Repeat(fmt.Sprintf("oldword%d ", i), 20)
			newContent = strings.Repeat(fmt.Sprintf("newword%d ", i), 15)
		}
		lines = append(lines,
			Line{Kind: LineRemoved, Content: oldContent, OldLine: &oldN},
			Line{Kind: LineAdded, Content: newContent, NewLine: &newN},
		)
	}
	return FileDiff{Path: path, Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: lines}}}
}

func TestWindowedSplitRenderMatchesFullRender(t *testing.T) {
	origMargin := diffRenderMarginRows
	defer func() { diffRenderMarginRows = origMargin }()

	fd := fileDiffWithVariedModifiedLines("a.go", 300)
	numSplitRows := len(flattenFileSplit(fd))

	for _, wrapLines := range []bool{false, true} {
		for _, cursorRow := range []int{0, numSplitRows / 2, numSplitRows - 1} {
			name := fmt.Sprintf("wrap=%v cursor=%d", wrapLines, cursorRow)
			t.Run(name, func(t *testing.T) {
				m := newModel("/repo", []FileDiff{fd}, Session{}, nil)
				m.wrapLines = wrapLines
				m.showLineNumbers = true
				m.lineIndex = cursorRow
				m.width, m.height = 100, 40
				width := 80

				diffRenderMarginRows = 5
				m.splitLayoutCache = map[string]diffLayout{}
				m.splitLinesCache = map[string]diffLinesCache{}
				wLines, wCursor, wRowFor, _ := m.buildSplitDiffLines(width)

				diffRenderMarginRows = 100000
				m.splitLayoutCache = map[string]diffLayout{}
				m.splitLinesCache = map[string]diffLinesCache{}
				fLines, fCursor, fRowFor, _ := m.buildSplitDiffLines(width)

				if len(wLines) != len(fLines) {
					t.Fatalf("total physical lines mismatch: windowed=%d full=%d", len(wLines), len(fLines))
				}
				if wCursor != fCursor {
					t.Fatalf("cursorLine mismatch: windowed=%d full=%d", wCursor, fCursor)
				}

				lo, hi := wCursor-5, wCursor+5
				if lo < 0 {
					lo = 0
				}
				if hi > len(wLines) {
					hi = len(wLines)
				}
				for i := lo; i < hi; i++ {
					if wLines[i] != fLines[i] {
						t.Errorf("line %d content mismatch\n  windowed: %q\n  full:     %q", i, wLines[i], fLines[i])
					}
					if wRowFor[i] != fRowFor[i] {
						t.Errorf("line %d rowFor mismatch: windowed=%d full=%d", i, wRowFor[i], fRowFor[i])
					}
				}
			})
		}
	}
}

// TestWrapLineCapUsesVisibleWidth guards wrapLine's size cap against
// checking ANSI-styled byte length instead of visible width — a heavily
// styled but visually short line could otherwise skip wrapping and get
// silently truncated downstream.
func TestWrapLineCapUsesVisibleWidth(t *testing.T) {
	var styled strings.Builder
	for i := 0; i < 200; i++ {
		styled.WriteString("\x1b[38;2;255;0;0mword\x1b[0m ")
	}
	s := styled.String()
	if len(s) <= maxHighlightLineChars {
		t.Fatalf("test line's byte length (%d) must exceed the cap (%d) to be meaningful", len(s), maxHighlightLineChars)
	}
	out := wrapLine(s, 40)
	if len(out) < 2 {
		t.Fatalf("expected a visually long line to wrap into multiple physical lines, got %d", len(out))
	}
}

// TestClampDiffScrollStaysWithinRenderedMargin guards manual scroll
// (model.diffScroll) against pushing the viewport past what
// buildDiffLinesDetailed actually renders (±diffRenderMarginRows from the
// cursor) — anything further lands on blank placeholder rows.
func TestClampDiffScrollStaysWithinRenderedMargin(t *testing.T) {
	height := 40
	bound := diffRenderMarginRows - height

	if got := clampDiffScroll(999999, height); got != bound {
		t.Errorf("expected large positive scroll clamped to %d, got %d", bound, got)
	}
	if got := clampDiffScroll(-999999, height); got != -bound {
		t.Errorf("expected large negative scroll clamped to %d, got %d", -bound, got)
	}
	if got := clampDiffScroll(5, height); got != 5 {
		t.Errorf("expected a small in-range scroll to pass through unchanged, got %d", got)
	}
}

func TestCapUnstyledLengthLeavesShortContentAlone(t *testing.T) {
	s := "short line"
	if got := capUnstyledLength(s, 40); got != s {
		t.Errorf("expected short content unchanged, got %q", got)
	}
}

func TestCapUnstyledLengthSlicesLongPlainContent(t *testing.T) {
	s := strings.Repeat("a", 100000)
	got := capUnstyledLength(s, 40)
	if len(got) >= len(s) {
		t.Fatalf("expected long plain content to be sliced shorter, got len %d", len(got))
	}
	if len(got) < 40 {
		t.Fatalf("expected sliced content to still cover the requested width, got len %d", len(got))
	}
}

// TestCapUnstyledLengthLeavesStyledContentAlone guards against slicing
// through the bytes of an ANSI escape sequence itself — here one starts a
// few bytes before the cut point, so a naive byte-slice would corrupt it.
func TestCapUnstyledLengthLeavesStyledContentAlone(t *testing.T) {
	width := 40
	limit := width * 8
	s := strings.Repeat("a", limit-5) + "\x1b[38;2;255;0;0m" + strings.Repeat("a", 100000)
	if got := capUnstyledLength(s, width); got != s {
		t.Error("expected content with an ANSI code straddling the cut point to be left unchanged")
	}
}

// TestFitLineMatchesUncappedForLongPlainLine proves capUnstyledLength's
// shortcut in fitLine doesn't change fitLine's actual output for a long
// plain line — same truncated/padded result, just without measuring the
// whole thing.
func TestFitLineMatchesUncappedForLongPlainLine(t *testing.T) {
	long := strings.Repeat("ab", 60000)
	want := ansi.Truncate(long, 40, "…")
	if w := ansi.StringWidth(want); w < 40 {
		want += strings.Repeat(" ", 40-w)
	}
	if got := fitLine(long, 40); got != want {
		t.Errorf("fitLine result changed by the cap:\n got:  %q\n want: %q", got, want)
	}
}
