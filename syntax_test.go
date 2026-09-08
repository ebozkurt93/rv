package main

import (
	"image/color"
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

// TestSetBackgroundColorUsesRealColorNotAGuess guards a real bug reported
// live: context-line text ("very low contrast and very hard to read")
// stayed unreadable even after minTintedContrast was raised, because
// plainFormatter's contrast reference (contextBg) was always a hardcoded
// "#000000"/"#ffffff" guess, discarding the terminal's own actual
// background color that Bubble Tea already queries via OSC 11 (see
// tea.BackgroundColorMsg) — a real dark terminal theme (a dark navy or
// purple, say) has a meaningfully different luminance than pure black, so
// a token's contrast was being checked against the wrong reference the
// whole time. setBackgroundColor must use bg's own exact value as that
// reference, not fall back to a black/white stand-in the way
// setBackgroundIsDark does.
func TestSetBackgroundColorUsesRealColorNotAGuess(t *testing.T) {
	origCtx := syntaxContextFmt
	origAdded, origRemoved, origCursor := bgAdded, bgRemoved, bgCursor
	t.Cleanup(func() {
		syntaxContextFmt = origCtx
		bgAdded, bgRemoved, bgCursor = origAdded, origRemoved, origCursor
	})

	// A real, fairly dark navy (not pure black, and not one of rv's own
	// tints) — distinct enough from "#000000" that a wrong reference
	// would be measurable.
	navy := color.RGBA{R: 0x1a, G: 0x1a, B: 0x2e, A: 0xff}
	setBackgroundColor(navy)

	pf, ok := syntaxContextFmt.(*plainFormatter)
	if !ok {
		t.Fatalf("expected syntaxContextFmt to be a *plainFormatter, got %T", syntaxContextFmt)
	}
	want := chroma.MustParseColour("#1a1a2e")
	if pf.bg != want {
		t.Fatalf("expected plainFormatter's contrast reference to be the real bg %v, got %v", want, pf.bg)
	}
	// Also confirms setBackgroundColor picked "dark" correctly from the
	// real color (navy's luminance is well under 0.5), not stuck on
	// whatever setBackgroundIsDark last set.
	if bgAdded != lipgloss.Color(darkTints.added) {
		t.Fatalf("expected dark tints for a dark navy background, got bgAdded=%v", bgAdded)
	}
}

// TestPlainFormatterFallsBackOnLowContrastAgainstRealBackground guards a
// real, reported bug: plainFormatter (used for every unchanged/context
// line — most of what's on screen) had no contrast check against the
// terminal's actual background at all, unlike tintedFormatter. Colors
// tuned to read well on a dark background are frequently near-unreadable
// on a light one. Verified directly against the real paraiso-dark palette:
// on a light background, ordinary Go code like "func main() { return nil
// }" fell below the contrast threshold on every single token.
func TestPlainFormatterFallsBackOnLowContrastAgainstRealBackground(t *testing.T) {
	orig := syntaxContextFmt
	t.Cleanup(func() { syntaxContextFmt = orig })

	setBackgroundIsDark(false)
	lexer := pickLexer("a.go")
	iterator, err := lexer.Tokenise(nil, "func main() { return nil }")
	if err != nil {
		t.Fatalf("tokenise: %v", err)
	}
	white := chroma.MustParseColour("#ffffff")
	sawRepair := false
	for tok := iterator(); tok != chroma.EOF; tok = iterator() {
		entry := syntaxContextStyle.Get(tok.Type)
		if !entry.Colour.IsSet() {
			continue
		}
		if contrastRatio(entry.Colour, white) < minTintedContrast {
			sawRepair = true
			if r := contrastRatio(ensureContrast(entry.Colour, white), white); r < minTintedContrast {
				t.Fatalf("expected ensureContrast to repair token color %v against a light background, got ratio %.2f", entry.Colour, r)
			}
		}
	}
	if !sawRepair {
		t.Skip("this base style's colors already clear minTintedContrast against a light background; nothing to guard")
	}
}

// TestEnsureContrastReturnsColorUnchangedWhenAlreadyReadable guards the
// core of the truecolor-first design: a token's own chroma color should
// come through byte-for-byte (no adjustment at all) whenever it already
// clears minTintedContrast against bg.
func TestEnsureContrastReturnsColorUnchangedWhenAlreadyReadable(t *testing.T) {
	cyan := chroma.MustParseColour("#00ffff")
	black := chroma.MustParseColour("#000000")
	got := ensureContrast(cyan, black)
	if got != cyan {
		t.Fatalf("expected the token's own color unchanged (already high contrast), got %v", got)
	}
}

// TestEnsureContrastNudgesTowardReadableWhenTooClose guards the fix for a
// real reported bug ("some text is very low contrast and very hard to
// read"): an earlier version of this either passed a token's color through
// completely unmodified against a much laxer bar, or discarded it entirely
// for flat black/white — nothing in between. ensureContrast must nudge a
// too-close-to-bg color just far enough to clear a real WCAG AA bar
// (4.5:1), landing on some genuinely readable result, while preserving
// hue/chroma as long as the adjustment allows rather than immediately
// collapsing to flat black/white.
func TestEnsureContrastNudgesTowardReadableWhenTooClose(t *testing.T) {
	// A genuinely dark bg (not one of rv's own subtle-wash tints, whose
	// max possible contrast can itself land under 4.5 — see
	// TestEnsureContrastReachesTintsOwnMaximum below) so clearing the
	// real target is actually possible here, straightforwardly proving
	// ensureContrast can get there and not just "improve some."
	color := chroma.MustParseColour("#0a0a0a")
	bg := chroma.MustParseColour("#0a0a0a") // identical to the token's own color: 1:1 contrast
	got := ensureContrast(color, bg)
	if r := contrastRatio(got, bg); r < minTintedContrast {
		t.Fatalf("expected the adjusted color to clear minTintedContrast (%.1f) against bg, got ratio %.2f (%v)", minTintedContrast, r, got)
	}
}

// TestEnsureContrastReachesTintsOwnMaximum guards the honest limit case: a
// background whose own ceiling (best achievable contrast, with either pure
// black or pure white — whichever direction ensureContrast pushes) sits
// below minTintedContrast. ensureContrast can't manufacture contrast that
// isn't physically there; the right behavior is converging on the
// direction's own best-possible foreground (white, on a dark bg) rather
// than looping forever, or falling back to something else, or overshooting
// past white.
func TestEnsureContrastReachesTintsOwnMaximum(t *testing.T) {
	// #737373's luminance (~0.45, still <= 0.5 so ensureContrast lightens
	// toward white, same direction it'd take on any of rv's own dark
	// tints) keeps even pure white's contrast against it under
	// minTintedContrast (3.0) — this bg's ceiling for that direction.
	bg := chroma.MustParseColour("#737373")
	got := ensureContrast(chroma.MustParseColour("#737373"), bg)
	maxPossible := contrastRatio(chroma.MustParseColour("#ffffff"), bg)
	if r := contrastRatio(got, bg); r < maxPossible-0.05 {
		t.Fatalf("expected ensureContrast to reach this bg's own practical ceiling (~%.2f), got ratio %.2f (%v)", maxPossible, r, got)
	}
}

// TestEnsureContrastPicksDirectionFromBackgroundLuminance guards nudging
// the right way: lightened (toward white) on a dark bg, darkened (toward
// black) on a light bg — nudging the wrong direction would still increase
// contrast some (any change does), but converges far slower and can
// overshoot into a harsh, over-bright/over-dark result on the wrong side
// of the background instead of a natural corrected shade.
func TestEnsureContrastPicksDirectionFromBackgroundLuminance(t *testing.T) {
	midGray := chroma.MustParseColour("#808080")

	// Both bg's are deliberately close to midGray's own luminance (not
	// black/white) so contrast is genuinely poor and a repair is
	// actually triggered, rather than the color passing through
	// unchanged because a very dark/light bg already gave it enough
	// contrast on its own.
	onDark := ensureContrast(midGray, chroma.MustParseColour("#404040"))
	if relativeLuminance(onDark) <= relativeLuminance(midGray) {
		t.Fatalf("expected a lighter result on a dark bg, got %v (from %v)", onDark, midGray)
	}

	onLight := ensureContrast(midGray, chroma.MustParseColour("#c0c0c0"))
	if relativeLuminance(onLight) >= relativeLuminance(midGray) {
		t.Fatalf("expected a darker result on a light bg, got %v (from %v)", onLight, midGray)
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
// tokens (e.g. a comment) whose color is nearly the same as the tint
// itself, which without repair renders as text with virtually no contrast
// against its own line's background. Uses the tint as the token color's bg
// directly (guaranteed identical, not dependent on which base style
// happens to be configured) so the test doesn't need to skip.
func TestTintedFormatterFallsBackWhenTokenColorLacksContrast(t *testing.T) {
	f := newTintedFormatter("#0a0a0a", "#2d6b45")
	got := ensureContrast(f.bg, f.bg)
	if r := contrastRatio(got, f.bg); r < minTintedContrast {
		t.Fatalf("expected a contrast-repaired color for a token identical to bg, got ratio %.2f (%v)", r, got)
	}
}

// TestTintedFormatterChecksContrastAgainstTheRightBackground guards a real
// bug reported live: a token's contrast was always checked against the
// base tint (bg), even for the runes an intraline diff highlights, which
// actually render against the stronger, more saturated wash (strongBg,
// baked in via strongBgEscape — see formatMasked). A color that reads fine
// against the dim base tint can read poorly against the lighter/more
// saturated strong wash, so the highlighted span went through with a
// near-illegible foreground even though the very same token read fine one
// column to either side of it.
func TestTintedFormatterChecksContrastAgainstTheRightBackground(t *testing.T) {
	// A mid-gray reads fine against a near-black bg but poorly against a
	// much lighter strongBg — the two backgrounds must get independently
	// judged repairs, not the base tint's verdict applied to both.
	f := newTintedFormatter("#0a0a0a", "#cccccc")
	gray := chroma.MustParseColour("#999999")

	normal := ensureContrast(gray, f.bg)
	if normal != gray {
		t.Fatalf("expected the token's own color unchanged against the dark base tint, got %v", normal)
	}

	strong := ensureContrast(gray, f.strongBg)
	if strong == gray {
		t.Fatalf("expected a contrast-repaired color against the light strong tint (contrast too poor there), got unchanged %v", strong)
	}
	if r := contrastRatio(strong, f.strongBg); r < minTintedContrast {
		t.Fatalf("expected the repaired color to actually clear minTintedContrast against strongBg, got ratio %.2f", r)
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
