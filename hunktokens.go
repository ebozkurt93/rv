package main

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
)

// applySyntaxTokens tokenizes h's old-side and new-side line sequences as
// two contiguous blocks — rather than tokenizing each Line's Content in
// isolation, one call per line — and slices the results back into each
// Line's own Tokens.
//
// A hunk's context+removed lines, in order, are a genuinely contiguous
// span of the file's old content; its context+added lines are a
// contiguous span of the new content. A lexer that needs state carried
// across lines (the canonical case: a "/* */" block comment, where a
// continuation line has no delimiter of its own to signal "I'm still
// inside a comment") only sees that correctly if it tokenizes the whole
// span in one call — chroma's Lexer.Tokenise takes arbitrary multi-line
// text and is stateful within a single call, so this is purely a matter
// of not throwing that context away by splitting into per-line calls
// before tokenizing. Verified directly: tokenizing a 3-line "/** ... */"
// block as one call correctly produces a single CommentMultiline token,
// where tokenizing each line alone had every word of the comment's body
// classified as if it were live code (keywords, constants, identifiers).
func applySyntaxTokens(h *Hunk, lexer chroma.Lexer) {
	var oldIdx, newIdx []int
	var oldLines, newLines []string
	for i, l := range h.Lines {
		// Tabs expanded before tokenizing (not after) so the resulting
		// Tokens' rune positions land exactly where renderLine's own
		// visibleContent (l.Content with the same expansion) expects them
		// — including matching expandMaskForTabs's already-expanded
		// intraline highlight mask. Expanding first doesn't shift where
		// the "\n" separators fall, so splitTokensByLine's per-line
		// splitting is unaffected.
		content := strings.ReplaceAll(l.Content, "\t", "    ")
		if l.Kind != LineAdded {
			oldIdx = append(oldIdx, i)
			oldLines = append(oldLines, content)
		}
		if l.Kind != LineRemoved {
			newIdx = append(newIdx, i)
			newLines = append(newLines, content)
		}
	}
	assignBlockTokens(h, lexer, oldIdx, oldLines)
	assignBlockTokens(h, lexer, newIdx, newLines)
}

// assignBlockTokens tokenizes lines (already joined by the caller's
// grouping — old-side or new-side) as one block and writes each
// resulting per-line token slice into h.Lines[idx[i]].Tokens. Leaves
// Tokens untouched (nil) on a tokenizer error, matching highlightContent's
// existing per-line-tokenize fallback for that case.
func assignBlockTokens(h *Hunk, lexer chroma.Lexer, idx []int, lines []string) {
	if len(idx) == 0 {
		return
	}
	iterator, err := lexer.Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return
	}
	perLine := splitTokensByLine(iterator, len(idx))
	for i, toks := range perLine {
		h.Lines[idx[i]].Tokens = toks
	}
}

// splitTokensByLine walks a flat token stream (as produced by tokenizing
// several lines' content joined with "\n") and distributes it into n
// per-line slices, splitting any token whose Value spans a line boundary
// into separate pieces that each keep the original token's Type. Each
// returned slice is non-nil (possibly empty, for a genuinely blank line)
// so callers can tell "tokenized, turned out empty" apart from "never
// tokenized at all" via a nil check.
func splitTokensByLine(iterator chroma.Iterator, n int) [][]chroma.Token {
	out := make([][]chroma.Token, n)
	for i := range out {
		out[i] = []chroma.Token{}
	}
	line := 0
	for tok := iterator(); tok != chroma.EOF; tok = iterator() {
		v := tok.Value
		for line < n {
			nl := strings.IndexByte(v, '\n')
			if nl < 0 {
				if v != "" {
					out[line] = append(out[line], chroma.Token{Type: tok.Type, Value: v})
				}
				break
			}
			if piece := v[:nl]; piece != "" {
				out[line] = append(out[line], chroma.Token{Type: tok.Type, Value: piece})
			}
			line++
			v = v[nl+1:]
		}
	}
	return out
}

// tokenIterator replays a pre-computed token slice as a chroma.Iterator,
// so highlightContent can format already-tokenized lines (see
// applySyntaxTokens) through the exact same formatting path it uses for
// the per-line-tokenize fallback.
func tokenIterator(tokens []chroma.Token) chroma.Iterator {
	i := 0
	return func() chroma.Token {
		if i >= len(tokens) {
			return chroma.EOF
		}
		t := tokens[i]
		i++
		return t
	}
}
