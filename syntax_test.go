package main

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestSetBackgroundIsDarkSwitchesTints guards the fix for a wrong startup
// guess (e.g. no $COLORFGBG set on an actually-light terminal) sticking
// for the whole session: once Bubble Tea reports the real background color
// via tea.BackgroundColorMsg, setBackgroundIsDark must actually swap the
// package-level tint state, not just the one it was initialized with.
func TestSetBackgroundIsDarkSwitchesTints(t *testing.T) {
	origAdded, origRemoved, origCursor := bgAdded, bgRemoved, bgCursor
	t.Cleanup(func() { bgAdded, bgRemoved, bgCursor = origAdded, origRemoved, origCursor })

	setBackgroundIsDark(true)
	if bgAdded != lipgloss.Color(darkTints.added) {
		t.Fatalf("expected dark tint after setBackgroundIsDark(true), got %v", bgAdded)
	}

	setBackgroundIsDark(false)
	if bgAdded != lipgloss.Color(lightTints.added) {
		t.Fatalf("expected light tint after setBackgroundIsDark(false), got %v", bgAdded)
	}
	if bgRemoved != lipgloss.Color(lightTints.removed) || bgCursor != lipgloss.Color(lightTints.cursor) {
		t.Fatalf("expected removed/cursor tints to switch too, got removed=%v cursor=%v", bgRemoved, bgCursor)
	}
}

// TestPlainFormatterFallsBackOnLowContrastAgainstRealBackground guards a
// real, reported bug: plainFormatter (used for every unchanged/context
// line — most of what's on screen) picked nearestANSI16 colors with no
// contrast check against the terminal's actual background at all, unlike
// tintedFormatter. Colors tuned to read well on a dark background (bright
// cyan, bright yellow — which is what most of a 16-slot reduction ends up
// using) are frequently near-unreadable on a light one. Verified directly
// against the real paraiso-dark palette: on a light background, ordinary
// Go code like "func main() { return nil }" fell below the contrast
// threshold on every single token, including plain whitespace.
func TestPlainFormatterFallsBackOnLowContrastAgainstRealBackground(t *testing.T) {
	orig := syntaxContextFmt
	t.Cleanup(func() { syntaxContextFmt = orig })

	setBackgroundIsDark(false)
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, syntaxContextFmt, "func main() { return nil }", nil, nil)
	if !strings.Contains(out, "\033[30m") {
		t.Fatalf("expected the black contrast fallback on a light background, got %q", out)
	}

	setBackgroundIsDark(true)
	out = highlightContent(lexer, syntaxContextFmt, "func main() { return nil }", nil, nil)
	if strings.Contains(out, "\033[30m") {
		t.Fatalf("expected no black fallback needed on a dark background (colors should already contrast fine), got %q", out)
	}
}

// TestNearestHueContrastingKeepsHueOnLightBackground guards the fix for
// an over-correction: an earlier version of the fix above made EVERY
// token fall back to flat black on a light background, since a token
// color close to a bright ANSI variant (tuned for dark backgrounds, which
// is what most of a 16-slot reduction favors) almost always fails
// contrast against white. That reads as "no syntax highlighting at all",
// not a fix. nearestHueContrasting instead picks the closest HUE first
// (checking both its normal and bright variant), then whichever of that
// hue's two variants actually contrasts with the background — so a color
// that's recognizably cyan, say, stays some shade of cyan on a light
// background instead of collapsing to black, as long as that hue has a
// variant dark enough to read on white.
func TestNearestHueContrastingKeepsHueOnLightBackground(t *testing.T) {
	white := chroma.MustParseColour("#ffffff")
	// #00ffff (bright cyan) is tuned for a dark background; its hue's
	// normal (dark cyan, #007f7f) variant should be picked here instead
	// of falling through to a fixed black/white.
	got := nearestHueContrasting(chroma.MustParseColour("#00ffff"), white)
	want := chroma.MustParseColour("#007f7f")
	if got != want {
		t.Fatalf("expected the dark-cyan variant %v on a light background, got %v", want, got)
	}
	if contrastRatio(got, white) < minTintedContrast {
		t.Fatalf("expected the picked variant to actually clear the contrast floor against white, got ratio %.2f", contrastRatio(got, white))
	}
}

// TestNearestHueContrastingNeverPicksPlainBlackOnDarkBackground guards a
// real bug reported live: an Operator token (gray #666666, e.g. "="/"=>")
// on rv's own dark-green added-line tint (#1f3d2b) rendered via plain ANSI
// "black" (SGR 30) — which nearestHueContrasting picked because its OWN
// reference hex (#000000) claims a healthy contrast ratio against
// #1f3d2b. That ratio is meaningless in practice: SGR 30 is the one slot
// virtually every dark terminal theme conventionally aliases to its own
// default background, so the operator was invisible even though the math
// said otherwise. On a dark background, the achromatic hue must always
// resolve to the bright/gray variant (SGR 90), never plain black.
func TestNearestHueContrastingNeverPicksPlainBlackOnDarkBackground(t *testing.T) {
	gray := chroma.MustParseColour("#666666")
	bgAddedDark := chroma.MustParseColour(darkTints.added)
	got := nearestHueContrasting(gray, bgAddedDark)
	if got == chroma.MustParseColour("#000000") {
		t.Fatalf("expected the bright/gray variant (never plain black, an unsafe slot on a dark bg), got %v", got)
	}
}

// TestNearestHueContrastingNeverPicksBrightWhiteOnLightBackground is the
// symmetric guard: on a light background, bright ANSI "white" (SGR 97) is
// the slot most light terminal themes alias to their own background, so
// the achromatic hue must resolve to the dimmer/gray variant (SGR 37)
// instead, never plain white.
func TestNearestHueContrastingNeverPicksBrightWhiteOnLightBackground(t *testing.T) {
	gray := chroma.MustParseColour("#999999")
	bgAddedLight := chroma.MustParseColour(lightTints.added)
	got := nearestHueContrasting(gray, bgAddedLight)
	if got == chroma.MustParseColour("#ffffff") {
		t.Fatalf("expected the dimmer/gray variant (never plain white, an unsafe slot on a light bg), got %v", got)
	}
}

// TestPlainFormatterNeverEmitsATokenBackground guards a real bug: monokai
// (like several chroma styles) sets a background on its "Error" token type,
// meant to flag a genuine lexer error. rv tokenizes one diff line at a time
// with no carried-over lexer state, so a bare block-comment delimiter like
// "*/" or a "*" continuation line reads as a syntax error in isolation and
// gets tokenized as Error — without this guard, that put a stray near-black
// box around otherwise-valid comment lines. Context lines should never show
// anything but the terminal's own background.
func TestPlainFormatterNeverEmitsATokenBackground(t *testing.T) {
	lexer := pickLexer("a.ts")
	bgEscape := regexp.MustCompile(`\x1b\[(48;|4[0-7]m|10[0-7]m)`)
	for _, line := range []string{" */", " /**", " *"} {
		out := highlightContent(lexer, syntaxContextFmt, line, nil, nil)
		if bgEscape.MatchString(out) {
			t.Fatalf("expected no background escape for context line %q, got %q", line, out)
		}
	}
}

func TestHighlightContentColorsDifferentTokenTypesDifferently(t *testing.T) {
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, syntaxContextFmt, "func main() {}", nil, nil)
	if out == "func main() {}" {
		t.Fatalf("expected ANSI-highlighted output, got plain text back: %q", out)
	}
}

func TestHighlightContentFallsBackOnFallbackLexer(t *testing.T) {
	lexer := pickLexer("no-such-extension.zzz")
	out := highlightContent(lexer, syntaxContextFmt, "plain text", nil, nil)
	if !strings.Contains(out, "plain text") {
		t.Fatalf("expected fallback lexer to still round-trip the content, got %q", out)
	}
}

func TestTintedFormatterBaksTruecolorBackgroundIntoEveryToken(t *testing.T) {
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, syntaxAddedFmt, "func main() {}", nil, nil)
	if !strings.Contains(out, "\033[48;2;") {
		t.Fatalf("expected a truecolor background escape in every token, got %q", out)
	}
}

// TestTintedFormatterFallsBackWhenTokenColorLacksContrast guards the fix for
// tokens (e.g. a comment) whose color reduces to nearly the same hue as the
// tint itself, which without a fallback renders as text with virtually no
// contrast against its own line's background.
func TestTintedFormatterFallsBackWhenTokenColorLacksContrast(t *testing.T) {
	f := newTintedFormatter("#1f3d2b", "#2d6b45") // same hue family as monokai's comment color
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, f, "// a comment", nil, nil)
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
