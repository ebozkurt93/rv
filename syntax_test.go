package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

// TestFitLineWithBackgroundCoversWrappedContinuation guards a real bug: a
// long single-token line (e.g. a whole-line comment) that word-wraps mid-
// token leaves its second physical line with no color codes of its own —
// the token's opening escape stayed on the first physical line — so once
// each physical line is composited independently into the bordered panel,
// the continuation rendered with no tint at all. fitLineWithBackground must
// prepend bg, not just append it for padding, so every physical line is
// self-contained regardless of where wrapLine happened to cut it.
func TestFitLineWithBackgroundCoversWrappedContinuation(t *testing.T) {
	continuation := "second half of a wrapped token, no leading color code"
	got := fitLineWithBackground(continuation, len(continuation)+5, bgAdded)
	if !strings.HasPrefix(got, "\033[48;2;") {
		t.Fatalf("expected the background escape to lead the line (not just trail as padding), got %q", got)
	}
	if !strings.HasSuffix(got, "\033[0m") {
		t.Fatalf("expected a trailing reset so the tint can't bleed into the border, got %q", got)
	}
}

// TestFitLineWithBackgroundPaddingItselfIsTinted guards a regression: s
// itself always ends with its own token's closing reset (every token is
// self-contained — open, text, reset), so prepending bg once at the very
// start covers the real content but does nothing for whatever comes after
// that trailing reset. Padding must carry its own bg escape too, or it
// renders as a plain, untinted gap between the text and the border.
func TestFitLineWithBackgroundPaddingItselfIsTinted(t *testing.T) {
	got := fitLineWithBackground("hi", 10, bgAdded)
	afterText := got[strings.Index(got, "hi")+len("hi"):]
	if !strings.Contains(afterText, "\033[48;2;") {
		t.Fatalf("expected the padding after the text to carry its own background escape (not rely on the leading one, which s's own trailing reset already canceled), got %q", got)
	}
}

// TestWrapLineDoesNotPreemptivelyPadWithPlainSpaces guards the actual root
// cause of a persistent "wrapped tinted lines have an untinted gap" bug:
// lipgloss.Style.Width() (which wrapLine uses to split long lines) doesn't
// just wrap — it also right-pads every resulting physical line to exactly
// width with plain, unstyled spaces. That meant a wrapped line arrived at
// fitLineWithBackground already at full width, so its "if w < width" padding
// branch (which tints the padding) never ran — the wrap-added padding
// stayed plain, uncolored space no matter what fitLineWithBackground did.
func TestWrapLineDoesNotPreemptivelyPadWithPlainSpaces(t *testing.T) {
	lines := wrapLine("short", 20)
	if len(lines) != 1 {
		t.Fatalf("expected exactly one line, got %d: %q", len(lines), lines)
	}
	if w := ansi.StringWidth(lines[0]); w >= 20 {
		t.Fatalf("expected wrapLine to leave short-line padding for the caller (fitLineWithBackground) to add, got %q (width %d)", lines[0], w)
	}
}

// TestRenderLineExpandsTabsBeforeMeasuringWidth guards a real "border
// visibly shifts" bug: ansi.StringWidth/ansi.Truncate (used throughout
// render.go for column-exact layout) count a literal tab as 0-1 cells,
// while a real terminal jumps it to the next tab-stop (up to 8 columns) —
// undercounting the true rendered width of any tab-indented line (i.e.
// almost every Go source line) and throwing off truncation/padding.
func TestRenderLineExpandsTabsBeforeMeasuringWidth(t *testing.T) {
	old := 1
	out := renderLine(pickLexer("a.go"), Line{Kind: LineContext, Content: "\treturn s", OldLine: &old, NewLine: &old}, false, 1, false)
	if strings.Contains(out, "\t") {
		t.Fatalf("expected the tab to be expanded to spaces before rendering, got %q", out)
	}
}

// TestWrappedTintedLineFullyCoversBothPhysicalLines is the end-to-end
// reproduction of the reported bug: a long comment on an added line, with
// wrap on, at a width that forces a mid-token break. Every physical line
// renderDiff produces for it — including the second one's leading edge and
// every line's trailing padding — must carry the tint.
func TestWrappedTintedLineFullyCoversBothPhysicalLines(t *testing.T) {
	withTempHome(t)

	new1 := 1
	fd := FileDiff{
		Path:   "x.go",
		Status: FileModified,
		Hunks: []Hunk{{Header: "h", Lines: []Line{
			{Kind: LineAdded, Content: "// bg is prepended before s itself, not just appended after for the padding", NewLine: &new1},
		}}},
	}
	m := newModel("/tmp/repo", []FileDiff{fd}, Session{}, nil)
	m.wrapLines = true
	m.width = 78
	m.height = 10

	out := m.renderDiff()
	rows := strings.Split(out, "\n")
	var contentRows []string
	for _, r := range rows {
		if strings.Contains(r, "\033[48;2;") {
			contentRows = append(contentRows, r)
		}
	}
	if len(contentRows) < 2 {
		t.Fatalf("expected the long comment to wrap into at least 2 tinted physical lines, got %d: %q", len(contentRows), out)
	}

	// This is the exact signature of the bug: wrapLine's own Style.Width()
	// closes an open style with a bare reset ("\x1b[m", no "0") right at
	// the word-wrap cut point, then pads with plain spaces — if nothing
	// re-applies the background between that bare reset and the line's
	// final reset, the gap in between renders with no color at all. A
	// single space right before the final reset is renderBorderedRaw's own
	// intentional 1-space margin before the border character, not a bug —
	// require 2+ to only catch a real unpainted padding run.
	broken := regexp.MustCompile(`\x1b\[m {2,}\x1b\[0m`)
	for _, r := range contentRows {
		if broken.MatchString(r) {
			t.Fatalf("found wrapLine's bare reset immediately followed by an unpainted gap: %q", r)
		}
	}
}

// TestRenderBorderedRawPreservesEmbeddedTruecolor guards against
// renderDiff going back to lipgloss.NewStyle().Border(...).Padding(0,1).
// Render(...) for content that's already pre-colored raw ANSI — lipgloss's
// own layout engine reprocesses embedded escapes when it computes padding,
// which silently drops a line's truecolor background. renderBorderedRaw
// must pass each content line through untouched (only concatenating
// independently-rendered border characters around it).
func TestRenderBorderedRawPreservesEmbeddedTruecolor(t *testing.T) {
	line := fitLineWithBackground("hi", 10, bgAdded)
	out := renderBorderedRaw(10, []string{line})
	if !strings.Contains(out, line) {
		t.Fatalf("expected the pre-colored line to appear byte-for-byte in the bordered output, got %q", out)
	}
}
