package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"charm.land/lipgloss/v2"
)

// baseSyntaxStyle is the chroma style whose per-token-type colors get
// reduced to the nearest of the basic 16 ANSI colors by formatters.TTY16 —
// so which base style we pick mostly just decides how tokens (keyword vs
// string vs comment...) sort into different ANSI slots; the *actual*
// rendered hue is always whatever the user's terminal theme has configured
// for that slot, same as the rest of rv's coloring.
const baseSyntaxStyle = "monokai"

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
// iTerm2, rxvt, konsole), without ever querying the terminal live (an OSC
// 11 "what's your background color" query can block for OSCTimeout==5s on
// a terminal that doesn't answer it, which is a bad startup-latency trade
// for a cosmetic tint). Defaults to dark, the far more common terminal
// setup, when the variable is absent or unparseable.
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

func activeTints() tintSet {
	if detectDarkBackground() {
		return darkTints
	}
	return lightTints
}

var (
	syntaxContextStyle *chroma.Style
	syntaxContextFmt   chroma.Formatter = formatters.TTY16
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
	tints := activeTints()
	syntaxAddedFmt = newTintedFormatter(tints.added, tints.addedStrong)
	syntaxRemovedFmt = newTintedFormatter(tints.removed, tints.removedStrong)
	syntaxCursorFmt = newTintedFormatter(tints.cursor, tints.cursor)
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

// highlightContent tokenizes content with lexer and formats it through fmt
// (plain formatters.TTY16 for context lines, or a tintedFormatter for
// added/removed/cursor lines — see renderLine), returning ANSI-escaped
// text. mask, when non-nil and fmtr is a *tintedFormatter, layers the
// formatter's strong tint over the runes it marks (see
// applyIntralineHighlights) — otherwise ignored. Falls back to plain
// content on any error (a lexer panic/failure shouldn't take the whole
// diff pane down with it).
func highlightContent(lexer chroma.Lexer, fmtr chroma.Formatter, content string, mask []bool) string {
	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
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
// considered unreadable against a tinted background and gets swapped for
// contrastFallback instead — e.g. monokai's comment gray, or a token
// that happens to reduce to the same hue as the tint itself (a dark green
// keyword on a dark green "added" tint), which without this would render
// as text with virtually no contrast against its own line.
const minTintedContrast = 2.2

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
			fg := nearestANSI16(entry.Colour)
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
