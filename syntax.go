package main

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// baseSyntaxStyle is the chroma style whose per-token-type colors get
// reduced to the nearest of the basic 16 ANSI colors by formatters.TTY16 —
// so which base style we pick mostly just decides how tokens (keyword vs
// string vs comment...) sort into different ANSI slots; the *actual*
// rendered hue is always whatever the user's terminal theme has configured
// for that slot, same as the rest of rv's coloring.
const baseSyntaxStyle = "monokai"

// Exact hex values from formatters' 16-color table (see chroma's
// tty_indexed.go), chosen deliberately so each tint lands on a specific
// ANSI slot rather than "nearest match to whatever I typed" — green(2) for
// added, red(1) for removed, bright-black(8) for the cursor row (which
// takes priority over a line's diff-status tint, matching how a "current
// line" highlight in most editors overrides other line-level decoration).
const (
	tintAddedHex   = "#007f00"
	tintRemovedHex = "#7f0000"
	tintCursorHex  = "#555555"
)

var (
	syntaxContextStyle *chroma.Style
	syntaxAddedStyle   *chroma.Style
	syntaxRemovedStyle *chroma.Style
	syntaxCursorStyle  *chroma.Style
)

func init() {
	base := styles.Get(baseSyntaxStyle)
	syntaxContextStyle = base
	syntaxAddedStyle = tintedStyle(base, tintAddedHex)
	syntaxRemovedStyle = tintedStyle(base, tintRemovedHex)
	syntaxCursorStyle = tintedStyle(base, tintCursorHex)
}

// tintedStyle returns a copy of base with the same per-token-type
// foreground colors but a uniform background baked into every entry.
// Baked in via chroma.StyleBuilder.Transform, not applied afterward by
// wrapping the *formatted* output in an outer style — chroma's own
// per-token formatting closes with a reset after every token (same as
// lipgloss's Render does), so an outer wrap would get canceled by the
// first token's reset, identical to the cursor-highlight nesting bug
// elsewhere in this file. Baking the background into the style itself
// means every token's own emitted escape sequence carries it, so there's
// nothing to cancel.
func tintedStyle(base *chroma.Style, bgHex string) *chroma.Style {
	bg := chroma.MustParseColour(bgHex)
	built, err := base.Builder().Transform(func(e chroma.StyleEntry) chroma.StyleEntry {
		e.Background = bg
		return e
	}).Build()
	if err != nil {
		return base
	}
	return built
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

// highlightContent tokenizes content with lexer and formats it through
// formatters.TTY16 using style, returning ANSI-escaped text. Falls back to
// plain content on any error (a lexer panic/failure shouldn't take the
// whole diff pane down with it).
func highlightContent(lexer chroma.Lexer, style *chroma.Style, content string) string {
	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
	}
	var buf strings.Builder
	if err := formatters.TTY16.Format(&buf, style, iterator); err != nil {
		return content
	}
	return buf.String()
}
