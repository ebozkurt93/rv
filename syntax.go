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
	colorful "github.com/lucasb-eyer/go-colorful"
)

// baseSyntaxStyle is the chroma style rv renders directly as truecolor (see
// plainFormatter/tintedFormatter) — same approach as bat/delta/GitHub's own
// diff view, rather than trying to remap syntax colors onto the user's
// 16-slot terminal ANSI palette (an earlier design this file used to use:
// it kept breaking in new ways every time a terminal theme reassigned one
// of those 16 slots to something unexpected, since there's no reliable way
// to know what a named ANSI color will actually render as). Rendering the
// style's own hex directly sidesteps that whole class of bug.
//
// friendly rather than the more common monokai: measured directly how many
// DISTINCT colors each candidate style produces for a realistic TypeScript
// sample. monokai collapses keywords, types, and constants all onto the
// same cyan, and names/functions/classes all onto the same yellow — e.g.
// "locker: Locker" renders the variable and its type annotation in the
// identical color. friendly keeps keywords and type names visually
// distinct without that collapsing.
const baseSyntaxStyle = "friendly"

// Added/removed/cursor tints are a subtle wash rather than a solid color —
// matching Hunk/GitHub's diff rendering. These are fixed truecolor hex
// values — picked once for a dark-background terminal (darkTints) or a
// light one (lightTints), chosen via a cheap COLORFGBG-based guess (see
// detectDarkBackground) since there's no way to read the terminal's actual
// background color without a live OSC query, which can hang for seconds on
// terminals that never answer it.
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
	contextBg := "#000000"
	if !dark {
		contextBg = "#ffffff"
	}
	applyBackground(dark, contextBg)
}

// setBackgroundColor is setBackgroundIsDark's more precise counterpart,
// used once Bubble Tea reports the terminal's ACTUAL background color (see
// update.go's tea.BackgroundColorMsg handler) — bg embeds color.Color
// directly, so it can be passed here as-is. plainFormatter's contrast
// reference (contextBg) then measures against this real color instead of a
// pure #000000/#ffffff stand-in: reported live, ordinary context-line text
// reading as low-contrast/hard to read even after minTintedContrast was
// raised, because the terminal's actual background (a dark navy, say) has
// a meaningfully different luminance than the "#000000" guess a token's
// contrast was actually being checked against — a color that cleared the
// bar against pure black doesn't necessarily clear it against the
// terminal's real, less-extreme background.
func setBackgroundColor(bg color.Color) {
	r, g, b, _ := bg.RGBA()
	hex := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	dark := relativeLuminance(chroma.MustParseColour(hex)) <= 0.5
	applyBackground(dark, hex)
}

// applyBackground is setBackgroundIsDark/setBackgroundColor's shared body:
// dark decides which tint set backs added/removed/cursor lines (see
// tintSet's own doc comment for why those stay fixed, deliberately subtle
// hues rather than also using the terminal's real color), while contextBg
// is the exact contrast reference plainFormatter checks context-line token
// colors against.
func applyBackground(dark bool, contextBg string) {
	tints := darkTints
	if !dark {
		tints = lightTints
	}
	syntaxAddedFmt = newTintedFormatter(tints.added, tints.addedStrong)
	syntaxRemovedFmt = newTintedFormatter(tints.removed, tints.removedStrong)
	syntaxCursorFmt = newTintedFormatter(tints.cursor, tints.cursor)
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

// relativeLuminance is a standard perceived-brightness weighting (WCAG's
// coefficients), used to judge a token's real chroma color against the
// exact background rv itself chose (context/added/removed/cursor tints are
// all fixed truecolor — see contrastRatio below).
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

// minTintedContrast is the WCAG contrast bar a token's color must clear
// against whatever background actually sits behind it — WCAG's "AA large
// text"/UI-component bar (3:1) rather than the stricter 4.5:1 body-text
// one, because 4.5:1 is mathematically unreachable against rv's OWN
// deliberately-subtle dark "added" tint (#1f3d2b, a wash, not solid — see
// tintSet's own doc comment) even with pure white text: measured directly,
// ~4.05 is the ceiling there, full stop, for any foreground at all. Picking
// a target above a real background's own ceiling wouldn't produce a better
// result — every token failing that background would converge on the
// exact same fully-desaturated white (see ensureContrast), which reads as
// "no syntax highlighting at all" on added lines specifically. 3:1 leaves
// real headroom under every one of rv's own tints' ceilings (~4.05 is the
// tightest), so most tokens needing repair land on a partial, still
// recognizably-tinted nudge instead of maxing out. Falling short doesn't
// mean discarding the color for flat black/white outright, unlike an
// earlier version of this file — ensureContrast nudges it just far enough
// to clear this bar (see its own doc comment).
const minTintedContrast = 3.0

// truecolorFgEscape returns c's raw 24-bit truecolor foreground escape —
// rendering a chroma style's own color directly rather than remapping it
// onto a named ANSI slot (see baseSyntaxStyle's doc comment for why).
func truecolorFgEscape(c chroma.Colour) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.Red(), c.Green(), c.Blue())
}

// ensureContrast returns seeking unchanged if it already clears
// minTintedContrast against bg, otherwise nudges its OkLab lightness
// (go-colorful's perceptually-uniform lightness/chroma/hue space, already
// an indirect dependency via lipgloss) toward black or white — whichever
// direction increases contrast against bg — in small steps until it clears
// the bar. This replaces an earlier all-or-nothing design (a token's color
// either passed a much lower bar completely unmodified, or got discarded
// entirely for flat black/white): reported live as "some text is very low
// contrast and very hard to read," measured directly — the real chroma
// style's own colors routinely landed around 1.5-2.5 against rv's diff
// tints, well under a real 4.5 AA bar. Nudging lightness instead keeps a
// token recognizably its own hue for as long as the adjustment allows,
// only converging on flat black/white at the extreme (lightness pushed all
// the way to 0 or 1) — which happens automatically here rather than
// needing a separate fallback path, since that's just where this loop
// naturally bottoms out for a color that's already very desaturated.
func ensureContrast(seeking, bg chroma.Colour) chroma.Colour {
	if contrastRatio(seeking, bg) >= minTintedContrast {
		return seeking
	}
	col := colorful.Color{R: float64(seeking.Red()) / 255, G: float64(seeking.Green()) / 255, B: float64(seeking.Blue()) / 255}
	l, a, b := col.OkLab()
	lighten := relativeLuminance(bg) <= 0.5 // dark bg: push toward white; light bg: push toward black
	// Interpolates the WHOLE OkLab coordinate toward true white (l=1,
	// a=0, b=0) or true black (l=0, a=0, b=0) as t goes 0→1 — not just
	// lightness with a,b held fixed. Nudging lightness alone hits the
	// sRGB gamut boundary well before white/black for any saturated
	// color (a,b far from 0 isn't representable at the extremes of l),
	// so Clamped() silently plateaus on a not-light/dark-enough color
	// while l keeps climbing past 1 with no further visible effect —
	// exactly the bug this fixes: a token stuck at a still-too-low
	// contrast because pure lightness pushing had already gamut-capped.
	// Shrinking a,b in step with l guarantees reaching true white/black
	// at t=1, so a contrast target under the maximum possible (i.e. any
	// realistic minTintedContrast against a bg that isn't itself already
	// extreme) is always reachable.
	for i := 1; i <= 50; i++ {
		t := float64(i) / 50
		l2 := l + t*(1-l)
		if !lighten {
			l2 = l - t*l
		}
		r, g, bl := colorful.OkLab(l2, a*(1-t), b*(1-t)).Clamped().RGB255()
		cand := chroma.NewColour(r, g, bl)
		if contrastRatio(cand, bg) >= minTintedContrast || t >= 1 {
			return cand
		}
	}
	return seeking // unreachable: t reaches 1 (true white/black) within the loop
}

// plainFormatter renders each token's foreground only — no background at
// all, regardless of what the chroma style says. This deliberately diverges
// from chroma's own formatters.TTY16-style formatters, which also honor a
// style entry's own Background — monokai sets one on its "Error" token
// type (a near-black box) meant to flag a genuine lexer error, but it fires
// here on perfectly valid code too, because rv tokenizes one diff line at a
// time with no carried-over lexer state. A line like a bare "*/" or a "*"
// continuation line of a block comment reads as a syntax error in
// isolation and got tokenized as Error, putting a stray black box around
// it. Since context lines should never show anything but the terminal's
// own background anyway, the simplest fix is to never emit a token
// background here at all.
type plainFormatter struct {
	// bg is a contrast REFERENCE only, standing in for "how light or dark
	// is the terminal's real background" — never actually emitted as a
	// background escape (see above). Without checking contrast against
	// this at all (the bug this fixes), a color tuned to read well on a
	// dark background stays exactly that color on a light terminal too,
	// where it's nearly unreadable; reported directly against
	// paraiso-dark's palette.
	bg chroma.Colour
}

func newPlainFormatter(bgHex string) *plainFormatter {
	return &plainFormatter{bg: chroma.MustParseColour(bgHex)}
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
			formatting += truecolorFgEscape(ensureContrast(entry.Colour, f.bg))
		}
		io.WriteString(w, formatting)
		io.WriteString(w, token.Value)
		io.WriteString(w, "\033[0m")
	}
	return nil
}

// tintedFormatter renders each token's foreground as the chroma style's own
// truecolor hex, contrast-repaired against this formatter's fixed
// background tint (see ensureContrast) and paired per-token — baked into
// every single token's own escape sequence rather than applied as an outer
// wrap, since chroma resets (\033[0m) after every token, which would cancel
// an outer wrap after the first one.
type tintedFormatter struct {
	bg             chroma.Colour
	strongBg       chroma.Colour
	bgEscape       string
	strongBgEscape string
}

func newTintedFormatter(bgHex, strongBgHex string) *tintedFormatter {
	bg := chroma.MustParseColour(bgHex)
	strongBg := chroma.MustParseColour(strongBgHex)
	return &tintedFormatter{
		bg:             bg,
		strongBg:       strongBg,
		bgEscape:       fmt.Sprintf("\033[48;2;%d;%d;%dm", bg.Red(), bg.Green(), bg.Blue()),
		strongBgEscape: fmt.Sprintf("\033[48;2;%d;%d;%dm", strongBg.Red(), strongBg.Green(), strongBg.Blue()),
	}
}

func (f *tintedFormatter) Format(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
	return f.formatMasked(w, style, it, nil)
}

// formatMasked is Format plus intraline highlighting: when mask marks a
// token's runes (indexed by rune position within the overall token stream)
// as changed, that run gets strongBgEscape instead of bgEscape, splitting
// the token's own write into sub-runs as needed. Contrast is repaired
// against whichever of bg/strongBg will actually render behind each
// specific run — not just bg unconditionally — since a token's color can
// read fine against the dim base tint but too poorly against the more
// saturated strong wash used for the actual highlighted span (reported
// live: a comma/token going near-illegible specifically inside the
// stronger intraline-diff highlight, even though it read fine right next
// to it on the same line's base tint). A nil mask (the Format path) always
// takes the single-write fast path, using the base tint throughout.
func (f *tintedFormatter) formatMasked(w io.Writer, style *chroma.Style, it chroma.Iterator, mask []bool) error {
	pos := 0
	for token := it(); token != chroma.EOF; token = it() {
		entry := style.Get(token.Type)
		attrs := ""
		if entry.Bold == chroma.Yes {
			attrs += "\033[1m"
		}
		if entry.Underline == chroma.Yes {
			attrs += "\033[4m"
		}
		if entry.Italic == chroma.Yes {
			attrs += "\033[3m"
		}
		var fgNormal, fgStrong string
		if entry.Colour.IsSet() {
			fgNormal = truecolorFgEscape(ensureContrast(entry.Colour, f.bg))
			fgStrong = truecolorFgEscape(ensureContrast(entry.Colour, f.strongBg))
		}

		runes := []rune(token.Value)
		if mask == nil {
			io.WriteString(w, attrs)
			io.WriteString(w, fgNormal)
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
			io.WriteString(w, attrs)
			if hl {
				io.WriteString(w, fgStrong)
				io.WriteString(w, f.strongBgEscape)
			} else {
				io.WriteString(w, fgNormal)
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
