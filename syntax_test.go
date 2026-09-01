package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestHighlightContentColorsDifferentTokenTypesDifferently(t *testing.T) {
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, syntaxContextFmt, "func main() {}")
	if out == "func main() {}" {
		t.Fatalf("expected ANSI-highlighted output, got plain text back: %q", out)
	}
}

func TestHighlightContentFallsBackOnFallbackLexer(t *testing.T) {
	lexer := pickLexer("no-such-extension.zzz")
	out := highlightContent(lexer, syntaxContextFmt, "plain text")
	if !strings.Contains(out, "plain text") {
		t.Fatalf("expected fallback lexer to still round-trip the content, got %q", out)
	}
}

func TestTintedFormatterBaksTruecolorBackgroundIntoEveryToken(t *testing.T) {
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, syntaxAddedFmt, "func main() {}")
	if !strings.Contains(out, "\033[48;2;") {
		t.Fatalf("expected a truecolor background escape in every token, got %q", out)
	}
}

func TestRenderLineCursorStyleTakesPriorityOverDiffStatus(t *testing.T) {
	old := 1
	added := renderLine(pickLexer("a.go"), Line{Kind: LineAdded, Content: "x", NewLine: &old}, false, 1, false)
	cursorAdded := renderLine(pickLexer("a.go"), Line{Kind: LineAdded, Content: "x", NewLine: &old}, false, 1, true)
	if added == cursorAdded {
		t.Fatalf("expected cursor styling to change output even on an added line")
	}
}

// TestRowBackgroundTintsFullRowWidth guards the fix for "cursor/diff-status
// highlight only covers the gutter, not the whole line" — the padding
// fitLineWithBackground adds past the text must carry the same background
// the text itself already has baked in via the chroma style, and comment/
// reply/editor lines attached under a row must NOT inherit that row's tint
// (they never had it baked into their own text).
func TestRowBackgroundTintsFullRowWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	old, new := 2, 2
	fd := FileDiff{
		Path:   "main.go",
		Status: FileModified,
		Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineRemoved, Content: "x", OldLine: &old},
			{Kind: LineAdded, Content: "y", NewLine: &new},
			{Kind: LineContext, Content: "z", OldLine: &old, NewLine: &new},
		}}},
	}
	rows := flattenFile(fd).rows // rows[0] is the hunk header; content starts at rows[1]

	bg, tinted := rowBackground(rows, []int{1}, []bool{true}, 0, -1)
	if !tinted || bg != bgRemoved {
		t.Fatalf("expected removed row tinted %v, got bg=%v tinted=%v", bgRemoved, bg, tinted)
	}

	bg, tinted = rowBackground(rows, []int{2}, []bool{true}, 0, -1)
	if !tinted || bg != bgAdded {
		t.Fatalf("expected added row tinted %v, got bg=%v tinted=%v", bgAdded, bg, tinted)
	}

	_, tinted = rowBackground(rows, []int{3}, []bool{true}, 0, -1)
	if tinted {
		t.Fatalf("expected context row to be untinted")
	}

	bg, tinted = rowBackground(rows, []int{3}, []bool{true}, 0, 3)
	if !tinted || bg != bgCursor {
		t.Fatalf("expected cursor row tinted regardless of diff status, got bg=%v tinted=%v", bg, tinted)
	}

	// A comment/reply/editor line mapped to a tinted row (mainLine=false)
	// must not itself be tinted.
	_, tinted = rowBackground(rows, []int{1}, []bool{false}, 0, -1)
	if tinted {
		t.Fatalf("expected a non-main-line row (comment/reply/editor) to stay untinted")
	}

	padded := fitLineWithBackground("hi", 10, bgRemoved)
	if !strings.Contains(padded, "\033[48;2;") {
		t.Fatalf("expected padding to carry a truecolor background escape code, got %q", padded)
	}
}
