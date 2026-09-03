package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// baseSyntaxStyle is the chroma style whose per-token-type colors get
// reduced to the nearest of the basic 16 ANSI colors by plainFormatter/
// tintedFormatter — so which base style we pick mostly just decides how
// tokens (keyword vs string vs comment...) sort into different ANSI slots;
// the *actual* rendered hue is always whatever the user's terminal theme
// has configured for that slot, same as the rest of rv's coloring.
//
// friendly rather than the more common monokai: measured directly how
// many DISTINCT, ACTUALLY-READABLE colors (i.e. after
// nearestHueContrasting + the minTintedContrast floor — not just raw
// ANSI-slot count, which turned out to be a poor predictor once
// background-contrast filtering is applied) each candidate style produces
// for a realistic TypeScript sample. monokai collapses keywords, types,
// and constants all onto the same cyan slot, and names/functions/classes
// all onto the same yellow slot — e.g. "locker: Locker" renders the
// variable and its type annotation in the identical color. An earlier
// pick, paraiso-dark, fixed that specific case but only survived
// background-contrast filtering with 4 distinct colors on a dark
// terminal and 3 on a light one for the same sample; friendly kept 6 on
// both, while still keeping keywords and type names visually distinct
// (paraiso-dark's one genuine win) — without the pitfalls of other
// wide-spreading candidates tried: vim and tango leave ordinary
// identifiers/function names uncolored entirely (same as plain text),
// fruity collapses names/operators/punctuation all onto plain white.
const baseSyntaxStyle = "friendly"

// Added/removed/cursor tints are a subtle wash rather than a solid color —
// matching Hunk/GitHub's diff rendering. That rules out the basic 16-color
// ANSI palette for backgrounds: every one of those 16 slots is a fully
// saturated color (there's no "dark green" among them, only "green"), so
// baking one in as a background always looks like a neon highlighter rather
// than a tint. These are fixed truecolor hex values instead — picked once
// for a dark-background terminal (darkTints) or a light one (lightTints),
// chosen via a cheap COLORFGBG-based guess (see detectDarkBackground) since
// there's no way to read the terminal's actual background color without a
// live OSC query, which can hang for seconds on terminals that never
// answer it. Token foreground colors are unaffected — those still reduce
// to the user's actual 16-slot ANSI palette via nearestANSI16, same as
// plain context lines.
type tintSet struct {
	added, removed, cursor string
	// addedStrong/removedStrong are a more saturated wash than
	// added/removed, layered on top of it for the runes an intraline diff
	// marks as actually changed within a modified line — a stronger tint
	// on top of a tint, keyed word-for-word, rather than a different color
	// (matching Hunk/GitHub's intraline highlighting, which shades the
	// same hue harder rather than switching palettes).
	addedStrong, removedStrong string
}

var (
	darkTints  = tintSet{added: "#1f3d2b", removed: "#3d1f24", cursor: "#333333", addedStrong: "#2d6b45", removedStrong: "#6b2d38"}
	lightTints = tintSet{added: "#d9f0d9", removed: "#f5d9d9", cursor: "#e0e0e0", addedStrong: "#a8dfa8", removedStrong: "#e8a8a8"}
)

// detectDarkBackground guesses whether the terminal has a dark or light
// background from $COLORFGBG ("fg;bg", set by many terminals — e.g.
// iTerm2, rxvt, konsole). This is only the *startup* guess, used to pick
// tints before the real answer is known — Bubble Tea separately queries
// the terminal for its actual background color (OSC 11) through its own
// event loop, which unlike a synchronous query has a built-in timeout and
// can't stall startup; see RequestBackgroundColor in cli.go/update.go and
// setBackgroundIsDark below, which replaces this guess once that answer
// arrives. Defaults to dark, the far more common terminal setup, when the
// variable is absent or unparseable.
func detectDarkBackground() bool {
	v := os.Getenv("COLORFGBG")
	if v == "" {
		return true
	}
	parts := strings.Split(v, ";")
	bg, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return true
	}
	// Only the two canonical "light" ANSI slots count as light; every other
	// index (including the accent colors) is treated as dark.
	return bg != 7 && bg != 15
}

var (
	syntaxContextStyle *chroma.Style
	syntaxContextFmt   chroma.Formatter
	syntaxAddedFmt     chroma.Formatter
	syntaxRemovedFmt   chroma.Formatter
	syntaxCursorFmt    chroma.Formatter

	// bgAdded/bgRemoved/bgCursor are the same tint hex values baked into
	// syntaxAddedFmt/syntaxRemovedFmt/syntaxCursorFmt, exposed as
	// lipgloss.Colors so render.go can apply the identical tint to the
	// gutter, the +/-/cursor prefix, and fitLineWithBackground's padding —
	// everything around the syntax-highlighted text needs to match it
	// exactly or the tint would visibly stop short of the row's edges.
	bgAdded, bgRemoved, bgCursor color.Color
)

func init() {
	syntaxContextStyle = styles.Get(baseSyntaxStyle)
	setBackgroundIsDark(detectDarkBackground())
}

// setBackgroundIsDark (re)builds the tinted formatters and their exposed
// lipgloss.Colors for the given dark/light background. Called once at
// startup with detectDarkBackground's guess, and again — from update.go's
// tea.BackgroundColorMsg handler — once Bubble Tea reports the terminal's
// actual background color, so a wrong startup guess (e.g. no $COLORFGBG
// set, on an actually-light terminal) gets corrected instead of producing
// dark tints against a light UI for the rest of the session.
func setBackgroundIsDark(dark bool) {
	tints := darkTints
	contextBg := "#000000"
	if !dark {
		tints = lightTints
		contextBg = "#ffffff"
	}
	syntaxAddedFmt = newTintedFormatter(tints.added, tints.addedStrong)
	syntaxRemovedFmt = newTintedFormatter(tints.removed, tints.removedStrong)
	syntaxCursorFmt = newTintedFormatter(tints.cursor, tints.cursor)
	// plainFormatter never emits a background of its own (see its doc
	// comment) — contextBg is only a contrast reference, standing in for
	// "roughly how light or dark the terminal's real background is",
	// exactly like tintedFormatter's own bg is used to judge contrast
	// against a real tint. Without this, a color like bright cyan or
	// bright yellow — picked because it reads well on a dark background,
	// which nearestANSI16's whole reference table implicitly assumes —
	// stays exactly that color on a light terminal too, where it's nearly
	// unreadable.
	syntaxContextFmt = newPlainFormatter(contextBg)
	bgAdded = lipgloss.Color(tints.added)
	bgRemoved = lipgloss.Color(tints.removed)
	bgCursor = lipgloss.Color(tints.cursor)
}

// tintBgEscape returns bg's raw truecolor background escape. Used instead
// of lipgloss's Style.Background(bg) for anything that needs to line up
// with tintedFormatter's own output (the prefix character, the gutter) —
// lipgloss downgrades a hex color to the nearest basic-16 ANSI color when
// its detected terminal profile is less than truecolor, but tintedFormatter
// always emits a raw truecolor escape regardless of profile (same as
// fitLineWithBackground), so a lipgloss-rendered segment could otherwise
// land on a visibly different shade than the syntax-highlighted text right
// next to it in the same row.
func tintBgEscape(bg color.Color) string {
	r, g, b, _ := bg.RGBA()
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

// ansiFgEscape converts one of rv's basic-16-color lipgloss.Color values
// (e.g. colorAdded == "2") to its raw SGR foreground escape, following the
// standard split — normal colors 0-7 are 30-37, bright colors 8-15 are
// 90-97 — so it can be combined with tintBgEscape into one manually-built
// escape sequence instead of going through lipgloss (see tintBgEscape).
func ansiFgEscape(c color.Color) string {
	bc, ok := c.(ansi.BasicColor)
	if !ok {
		return ""
	}
	idx := int(bc)
	if idx < 8 {
		return fmt.Sprintf("\033[%dm", 30+idx)
	}
	return fmt.Sprintf("\033[%dm", 90+(idx-8))
}

// pickLexer resolves which chroma lexer to use for path, once per file
// (cached on fileRows — lexers.Match does filename pattern matching, not
// worth repeating per line).
func pickLexer(path string) chroma.Lexer {
	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

// highlightContent formats content through fmt (plainFormatter for
// context lines, or a tintedFormatter for added/removed/cursor lines —
// see renderLine), returning ANSI-escaped text. mask, when non-nil and
// fmtr is a *tintedFormatter, layers the formatter's strong tint over the
// runes it marks (see applyIntralineHighlights) — otherwise ignored.
//
// tokens, when non-nil, is used directly instead of tokenizing content —
// pre-computed for the whole hunk-side block content belongs to (see
// applySyntaxTokens), so a lexer needing cross-line state (a block
// comment spanning several lines, say) classifies it correctly instead of
// tokenizing this one line blind. Falls back to tokenizing content alone
// with lexer when tokens is nil (e.g. a synthetic FileDiff that never
// went through applySyntaxTokens) or on any tokenizer/formatter error —
// a lexer panic/failure shouldn't take the whole diff pane down with it.
func highlightContent(lexer chroma.Lexer, fmtr chroma.Formatter, content string, tokens []chroma.Token, mask []bool) string {
	iterator := tokenIterator(tokens)
	if tokens == nil {
		var err error
		iterator, err = lexer.Tokenise(nil, content)
		if err != nil {
			return content
		}
	}
	var buf strings.Builder
	var ferr error
	if tf, ok := fmtr.(*tintedFormatter); ok && mask != nil {
		ferr = tf.formatMasked(&buf, syntaxContextStyle, iterator, mask)
	} else {
		ferr = fmtr.Format(&buf, syntaxContextStyle, iterator)
	}
	if ferr != nil {
		return content
	}
	// Some lexers' comment rules match through end-of-line, so tokenising a
	// single line with no trailing "\n" (every diff line, always exactly one
	// line) can still emit one anyway — e.g. a "//" comment as the line's
	// last token, with the "\n" baked into that token's own Value. Left in,
	// that literal newline splits renderLine's return value across two
	// physical terminal rows once boxed up, which shifts the panel's right
	// border onto a phantom row below it (only visible in wrap mode, since
	// unwrapped rows get silently cut short by fitLine's truncation
	// instead). Stripped outright rather than trimmed from the end only —
	// tintedFormatter (see below) writes its per-token reset code right
	// after the token's own Value, so the "\n" ends up before that reset,
	// not at the very end of the buffer. content itself never legitimately
	// contains one.
	return strings.ReplaceAll(buf.String(), "\n", "")
}

// ansi16 is chroma's own 16-color TTY table (see its formatters/
// tty_indexed.go), copied here because it's unexported there — needed so
// tintedFormatter can emit the same theme-following 16-color foreground
// codes as formatters.TTY16 while pairing them with a truecolor background
// tint, which chroma's own formatters can't do (each only handles one
// color depth for both fg and bg).
var ansi16 = map[chroma.Colour]string{
	chroma.MustParseColour("#000000"): "\033[30m", chroma.MustParseColour("#7f0000"): "\033[31m",
	chroma.MustParseColour("#007f00"): "\033[32m", chroma.MustParseColour("#7f7fe0"): "\033[33m",
	chroma.MustParseColour("#00007f"): "\033[34m", chroma.MustParseColour("#7f007f"): "\033[35m",
	chroma.MustParseColour("#007f7f"): "\033[36m", chroma.MustParseColour("#e5e5e5"): "\033[37m",
	chroma.MustParseColour("#555555"): "\033[90m", chroma.MustParseColour("#ff0000"): "\033[91m",
	chroma.MustParseColour("#00ff00"): "\033[92m", chroma.MustParseColour("#ffff00"): "\033[93m",
	chroma.MustParseColour("#0000ff"): "\033[94m", chroma.MustParseColour("#ff00ff"): "\033[95m",
	chroma.MustParseColour("#00ffff"): "\033[96m", chroma.MustParseColour("#ffffff"): "\033[97m",
}

// nearestANSI16 finds the closest of chroma's 16 canonical colors to seeking
// (Lab-space distance, same metric formatters.TTY16 uses internally).
func nearestANSI16(seeking chroma.Colour) chroma.Colour {
	var closest chroma.Colour
	best := -1.0
	for candidate := range ansi16 {
		d := candidate.Distance(seeking)
		if best < 0 || d < best {
			best = d
			closest = candidate
		}
	}
	return closest
}

// ansiHue is chroma's own 16-color table (ansi16 above) regrouped into its
// 8 underlying hues, each with a "normal" (SGR 30-37) and "bright" (SGR
// 90-97) variant — e.g. dark red vs bright red. Used by
// nearestHueContrasting instead of nearestANSI16 for anything that needs
// to also stay readable against a known background: nearestANSI16 alone
// picks whichever of all 16 is objectively closest with no regard for
// which variant that happens to be, so a token whose real color is a
// vivid, bright hue (tuned to read well on a dark terminal, which is what
// most of a 16-slot reduction ends up favoring) stays exactly that bright
// on a light terminal too — where it's nearly unreadable. Falling back to
// a fixed black/white in that case (an earlier version of this fix) does
// solve readability, but at the cost of nearly all distinct token colors
// collapsing onto the same one or two colors, which reads as "no syntax
// highlighting" rather than "a fixed palette that respects the terminal."
var ansiHue = [8]struct{ normal, bright chroma.Colour }{
	{chroma.MustParseColour("#000000"), chroma.MustParseColour("#555555")},
	{chroma.MustParseColour("#7f0000"), chroma.MustParseColour("#ff0000")},
	{chroma.MustParseColour("#007f00"), chroma.MustParseColour("#00ff00")},
	{chroma.MustParseColour("#7f7fe0"), chroma.MustParseColour("#ffff00")},
	{chroma.MustParseColour("#00007f"), chroma.MustParseColour("#0000ff")},
	{chroma.MustParseColour("#7f007f"), chroma.MustParseColour("#ff00ff")},
	{chroma.MustParseColour("#007f7f"), chroma.MustParseColour("#00ffff")},
	{chroma.MustParseColour("#e5e5e5"), chroma.MustParseColour("#ffffff")},
}

// nearestHueContrasting finds which of ansiHue's 8 hues seeking is closest
// to (checking both variants' distance, so a color that happens to be
// vivid doesn't get compared unfairly against only the dark variants), then
// returns whichever of THAT hue's normal/bright variant contrasts better
// against bg — except for the achromatic hue (index 0, black/gray) on a
// dark bg, or the white hue (index 7) on a light bg, where the variant that
// would win on paper is skipped outright regardless of its computed
// contrast ratio: see contrastRatio's own reference-hex caveat below and
// pickAchromaticVariant's doc comment for why. The result still might not
// clear minTintedContrast — callers keep their own black/white fallback for
// that rarer case (e.g. a custom mid-gray background where neither variant
// of the matched hue reads well) — but for the common case (a genuinely
// light or dark background), this keeps text recognizably its own hue
// instead of needing that fallback at all.
func nearestHueContrasting(seeking, bg chroma.Colour) chroma.Colour {
	var closestIdx int
	var closest struct{ normal, bright chroma.Colour }
	best := -1.0
	for i, h := range ansiHue {
		d := h.normal.Distance(seeking)
		if db := h.bright.Distance(seeking); db < d {
			d = db
		}
		if best < 0 || d < best {
			best = d
			closest = h
			closestIdx = i
		}
	}
	if v, ok := pickAchromaticVariant(closestIdx, closest.normal, closest.bright, bg); ok {
		return v
	}
	if contrastRatio(closest.bright, bg) >= contrastRatio(closest.normal, bg) {
		return closest.bright
	}
	return closest.normal
}

// pickAchromaticVariant guards a real bug: plain ANSI "black" (SGR 30) and
// bright ANSI "white" (SGR 97) are the two terminal-theme slots virtually
// every color scheme conventionally aliases to its own default background
// — dark themes commonly set SGR 30 equal to (or barely lighter than) the
// terminal's own dark background, exactly the way light themes commonly do
// the same for SGR 97 against a light background. contrastRatio's math only
// checks OUR OWN reference hex (#000000/#ffffff) against OUR OWN fixed tint
// — it has no way to know the terminal will actually render that slot as
// something else entirely, so it can (and did: an operator token rendered
// via SGR 30 on rv's own dark-green added-line tint) claim a perfectly
// healthy contrast ratio for a combination that's invisible in practice.
// Since a dark bg only ever risks the SGR 30 slot, and a light bg only ever
// risks SGR 97, forcing the OTHER variant of that same hue pair sidesteps
// the unsafe slot entirely rather than trying to out-calculate a terminal
// theme this code can't see. ok is false for every hue but the two
// achromatic ones (0 and 7), where the general contrast-based pick in
// nearestHueContrasting already runs the caller does need instead.
func pickAchromaticVariant(hueIdx int, normal, bright, bg chroma.Colour) (chroma.Colour, bool) {
	dark := relativeLuminance(bg) <= 0.5
	switch {
	case hueIdx == 0 && dark: // black/gray hue, dark bg: SGR 30 is unsafe
		return bright, true
	case hueIdx == 7 && !dark: // white/gray hue, light bg: SGR 97 is unsafe
		return normal, true
	}
	return normal, false
}

// relativeLuminance is a standard perceived-brightness weighting (WCAG's
// coefficients), used only to compare our own fixed hex values against each
// other — not a claim about how any given terminal actually renders an ANSI
// slot, since that's terminal-theme-defined and unknowable here. Good
// enough as a heuristic: it's built from the same reference hexes
// nearestANSI16 already matches against.
func relativeLuminance(c chroma.Colour) float64 {
	return 0.2126*float64(c.Red())/255 + 0.7152*float64(c.Green())/255 + 0.0722*float64(c.Blue())/255
}

// contrastRatio is the WCAG contrast formula: (L1+0.05)/(L2+0.05) with L1
// the lighter of the two.
func contrastRatio(a, b chroma.Colour) float64 {
	la, lb := relativeLuminance(a)+0.05, relativeLuminance(b)+0.05
	if la < lb {
		la, lb = lb, la
	}
	return la / lb
}

// minTintedContrast is the floor below which a token's own color is
// considered unreadable against a tinted (or plain) background and gets
// swapped for the flat black/white fallback instead — e.g. monokai's
// comment gray, or a token that happens to reduce to the same hue as the
// tint itself (a dark green keyword on a dark green "added" tint), which
// without this would render as text with virtually no contrast against
// its own line.
//
// Deliberately well below a WCAG body-text bar (AA is 4.5:1, even
// AA-large is 3:1): chroma's own "normal" (non-bright) ANSI reference
// colors — the ones nearestHueContrasting picks for a light background —
// only have a luminance around 0.15-0.4 to begin with, so their contrast
// against a light background (or a pale tint like the "added"/"removed"
// wash) lands in the 1.6-2.4 range even in the best case. A stricter
// floor here doesn't produce BETTER color choices, since
// nearestHueContrasting has already picked the more-contrasting of a
// hue's two variants — it just rejects them anyway and forces flat
// black/white for nearly everything, which reads as "no syntax
// highlighting at all" (measured directly: 2.2 rejected 45 of 46 tokens
// in a real code sample against the "added" tint). This only guards
// against genuinely bad cases (plain text/punctuation colors that are
// already close to the background, typically under 1.2) — accent colors
// with merely modest contrast are still far more useful than no color at
// all, especially since anything rejected here still falls back to a
// clearly-readable flat black/white rather than disappearing.
const minTintedContrast = 1.5

// plainFormatter renders each token's foreground only — no background at
// all, regardless of what the chroma style says — reducing colors to the
// nearest of the basic 16 ANSI colors the same way tintedFormatter does,
// with the same black/white contrast fallback tintedFormatter uses too
// (see newPlainFormatter). This deliberately diverges from chroma's own
// formatters.TTY16 (which plainFormatter otherwise mirrors): TTY16 also
// honors a style entry's own Background, and monokai sets one on its
// "Error" token type (a near-black box) — meant to flag a genuine lexer
// error, but it fires here on perfectly valid code too, because rv
// tokenizes one diff line at a time with no carried-over lexer state. A
// line like a bare "*/" or a "*" continuation line of a block comment
// reads as a syntax error in isolation and got tokenized as Error,
// putting a stray black box around it. Since context lines should never
// show anything but the terminal's own background anyway, the simplest
// fix is to never emit a token background here at all.
type plainFormatter struct {
	// bg is a contrast REFERENCE only, standing in for "how light or dark
	// is the terminal's real background" — never actually emitted as a
	// background escape (see above). Without checking contrast against
	// this at all (the bug this fixes), a color like bright cyan or
	// bright yellow — picked because it reads well on a dark terminal,
	// which nearestANSI16's own reference table implicitly assumes —
	// stays exactly that color on a light terminal too, where it's nearly
	// unreadable; reported directly against paraiso-dark's palette.
	bg       chroma.Colour
	fallback chroma.Colour
}

func newPlainFormatter(bgHex string) *plainFormatter {
	bg := chroma.MustParseColour(bgHex)
	fallback := chroma.MustParseColour("#ffffff")
	if relativeLuminance(bg) > 0.5 {
		fallback = chroma.MustParseColour("#000000")
	}
	return &plainFormatter{bg: bg, fallback: fallback}
}

func (f *plainFormatter) Format(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
	for token := it(); token != chroma.EOF; token = it() {
		entry := style.Get(token.Type)
		formatting := ""
		if entry.Bold == chroma.Yes {
			formatting += "\033[1m"
		}
		if entry.Underline == chroma.Yes {
			formatting += "\033[4m"
		}
		if entry.Italic == chroma.Yes {
			formatting += "\033[3m"
		}
		if entry.Colour.IsSet() {
			fg := nearestHueContrasting(entry.Colour, f.bg)
			if contrastRatio(fg, f.bg) < minTintedContrast {
				fg = f.fallback
			}
			formatting += ansi16Or24(fg)
		}
		io.WriteString(w, formatting)
		io.WriteString(w, token.Value)
		io.WriteString(w, "\033[0m")
	}
	return nil
}

// tintedFormatter renders each token's foreground reduced to the nearest of
// the basic 16 ANSI colors (so it still follows the terminal theme, exactly
// like formatters.TTY16) — unless that color's own reference hex has too
// little contrast against this formatter's fixed background tint, in which
// case it falls back to whichever of black/white contrasts better against
// that tint. The background itself is paired in per-token, baked into every
// single token's own escape sequence rather than applied as an outer wrap:
// chroma resets (\033[0m) after every token, so an outer wrap would get
// canceled by the first token's reset.
type tintedFormatter struct {
	bg             chroma.Colour
	bgEscape       string
	strongBgEscape string
	fallback       chroma.Colour
}

func newTintedFormatter(bgHex, strongBgHex string) *tintedFormatter {
	bg := chroma.MustParseColour(bgHex)
	strongBg := chroma.MustParseColour(strongBgHex)
	fallback := chroma.MustParseColour("#ffffff")
	if relativeLuminance(bg) > 0.5 {
		fallback = chroma.MustParseColour("#000000")
	}
	return &tintedFormatter{
		bg:             bg,
		bgEscape:       fmt.Sprintf("\033[48;2;%d;%d;%dm", bg.Red(), bg.Green(), bg.Blue()),
		strongBgEscape: fmt.Sprintf("\033[48;2;%d;%d;%dm", strongBg.Red(), strongBg.Green(), strongBg.Blue()),
		fallback:       fallback,
	}
}

func (f *tintedFormatter) Format(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
	return f.formatMasked(w, style, it, nil)
}

// formatMasked is Format plus intraline highlighting: when mask marks a
// token's runes (indexed by rune position within the overall token stream)
// as changed, that run of the token gets strongBgEscape instead of
// bgEscape, splitting the token's own write into sub-runs as needed. A nil
// mask (the Format path) always takes the single-write fast path.
func (f *tintedFormatter) formatMasked(w io.Writer, style *chroma.Style, it chroma.Iterator, mask []bool) error {
	pos := 0
	for token := it(); token != chroma.EOF; token = it() {
		entry := style.Get(token.Type)
		formatting := ""
		if entry.Bold == chroma.Yes {
			formatting += "\033[1m"
		}
		if entry.Underline == chroma.Yes {
			formatting += "\033[4m"
		}
		if entry.Italic == chroma.Yes {
			formatting += "\033[3m"
		}
		if entry.Colour.IsSet() {
			fg := nearestHueContrasting(entry.Colour, f.bg)
			if contrastRatio(fg, f.bg) < minTintedContrast {
				fg = f.fallback
			}
			formatting += ansi16Or24(fg)
		}

		runes := []rune(token.Value)
		if mask == nil {
			io.WriteString(w, formatting)
			io.WriteString(w, f.bgEscape)
			io.WriteString(w, token.Value)
			io.WriteString(w, "\033[0m")
			pos += len(runes)
			continue
		}
		for start := 0; start < len(runes); {
			hl := pos+start < len(mask) && mask[pos+start]
			end := start + 1
			for end < len(runes) && (pos+end < len(mask) && mask[pos+end]) == hl {
				end++
			}
			io.WriteString(w, formatting)
			if hl {
				io.WriteString(w, f.strongBgEscape)
			} else {
				io.WriteString(w, f.bgEscape)
			}
			io.WriteString(w, string(runes[start:end]))
			io.WriteString(w, "\033[0m")
			start = end
		}
		pos += len(runes)
	}
	return nil
}

// ansi16Or24 returns c's escape from the 16-color table if c is one of
// those 16 canonical colors, otherwise (the black/white contrast fallback,
// which isn't necessarily one of them) a truecolor escape for c directly.
func ansi16Or24(c chroma.Colour) string {
	if esc, ok := ansi16[c]; ok {
		return esc
	}
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.Red(), c.Green(), c.Blue())
}
