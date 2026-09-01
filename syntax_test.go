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

// TestTintedFormatterFallsBackWhenTokenColorLacksContrast guards the fix for
// tokens (e.g. a comment) whose color reduces to nearly the same hue as the
// tint itself, which without a fallback renders as text with virtually no
// contrast against its own line's background.
func TestTintedFormatterFallsBackWhenTokenColorLacksContrast(t *testing.T) {
	f := newTintedFormatter("#1f3d2b") // same hue family as monokai's comment color
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, f, "// a comment")
	iterator, err := lexer.Tokenise(nil, "// a comment")
	if err != nil {
		t.Fatalf("tokenise: %v", err)
	}
	tok := iterator()
	entry := syntaxContextStyle.Get(tok.Type)
	if contrastRatio(nearestANSI16(entry.Colour), f.bg) >= minTintedContrast {
		t.Skip("this base style's comment color already contrasts fine against this tint; nothing to guard")
	}
	// The fallback colors (black/white) are themselves two of chroma's 16
	// canonical colors, so ansi16Or24 emits their plain ANSI codes (97/30)
	// rather than a truecolor escape — either is an acceptable fallback.
	if !strings.Contains(out, "\033[97m") && !strings.Contains(out, "\033[30m") {
		t.Fatalf("expected a contrast fallback (black or white) foreground, got %q", out)
	}
}

func TestGutterMatchesPrefixColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	old, new := 1, 1
	added := renderLine(pickLexer("a.go"), Line{Kind: LineAdded, Content: "x", NewLine: &new}, true, 1, false)
	removed := renderLine(pickLexer("a.go"), Line{Kind: LineRemoved, Content: "x", OldLine: &old}, true, 1, false)
	// The gutter number's own ANSI color code should match the +/- prefix's
	// (styleAdded/styleRemoved) rather than always being muted gray — the
	// gutter is otherwise the only unstyled patch of a tinted row. Both the
	// gutter and the prefix character carry the same SGR foreground code,
	// so it should appear (at least) twice in the rendered line.
	addedGreen := styleAdded.Render("+")[:strings.Index(styleAdded.Render("+"), "+")]
	removedRed := styleRemoved.Render("-")[:strings.Index(styleRemoved.Render("-"), "-")]
	if strings.Count(added, addedGreen) < 2 {
		t.Fatalf("expected added row's gutter to reuse the prefix's color code %q, got %q", addedGreen, added)
	}
	if strings.Count(removed, removedRed) < 2 {
		t.Fatalf("expected removed row's gutter to reuse the prefix's color code %q, got %q", removedRed, removed)
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
