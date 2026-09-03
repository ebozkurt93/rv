package main

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
)

// TestApplySyntaxTokensClassifiesBlockCommentCorrectly guards the actual
// bug reported: "random text in comments gets highlighted". A lexer
// tokenizing one diff line at a time has no way to know a line is the
// continuation of a "/* */" block comment, since there's no delimiter on
// that line by itself — so words like "if"/"true"/"null" got classified
// as live keywords/constants instead of comment text. Tokenizing each
// hunk side as one contiguous block (what applySyntaxTokens does) fixes
// this because chroma's lexer is stateful within a single Tokenise call.
func TestApplySyntaxTokensClassifiesBlockCommentCorrectly(t *testing.T) {
	n := 1
	lines := make([]Line, 0)
	for _, c := range []string{"/**", " * if this is true, return null.", " */"} {
		nn := n
		lines = append(lines, Line{Kind: LineContext, Content: c, OldLine: &nn, NewLine: &nn})
		n++
	}
	h := Hunk{Lines: lines}
	applySyntaxTokens(&h, pickLexer("a.ts"))

	for _, l := range h.Lines {
		if len(l.Tokens) == 0 {
			t.Fatalf("expected tokens to be populated for line %q", l.Content)
		}
		for _, tok := range l.Tokens {
			if tok.Type != chroma.CommentMultiline {
				t.Fatalf("expected every token in a block comment to be CommentMultiline, got %s (%q) in line %q",
					tok.Type, tok.Value, l.Content)
			}
		}
	}
}

// TestApplySyntaxTokensLeavesCodeLinesAlone is the sibling guard: real
// code content (not inside any comment) should still classify normally —
// this isn't about forcing everything to look like a comment, only about
// carrying real cross-line lexer state.
func TestApplySyntaxTokensLeavesCodeLinesAlone(t *testing.T) {
	n := 1
	line := Line{Kind: LineContext, Content: "const x = 1;", OldLine: &n, NewLine: &n}
	h := Hunk{Lines: []Line{line}}
	applySyntaxTokens(&h, pickLexer("a.ts"))

	foundKeyword := false
	for _, tok := range h.Lines[0].Tokens {
		if tok.Type == chroma.KeywordReserved && tok.Value == "const" {
			foundKeyword = true
		}
	}
	if !foundKeyword {
		t.Fatalf("expected \"const\" to still classify as a keyword outside a comment, got %+v", h.Lines[0].Tokens)
	}
}

// TestApplySyntaxTokensOldAndNewSidesAreIndependent guards that a removed
// line only draws cross-line context from other old-side (context+
// removed) lines, and an added line only from other new-side (context+
// added) lines — not from each other, since they don't actually sit next
// to each other in either version of the file.
func TestApplySyntaxTokensOldAndNewSidesAreIndependent(t *testing.T) {
	old1, new1 := 1, 1
	h := Hunk{Lines: []Line{
		{Kind: LineRemoved, Content: "/* start of a comment", OldLine: &old1},
		{Kind: LineAdded, Content: "const totallyUnrelated = 1;", NewLine: &new1},
	}}
	applySyntaxTokens(&h, pickLexer("a.go"))

	// The added line must NOT be swept into the removed line's unterminated
	// comment — it should tokenize as normal Go code.
	foundKeyword := false
	for _, tok := range h.Lines[1].Tokens {
		if tok.Type == chroma.KeywordDeclaration || tok.Type == chroma.Keyword {
			foundKeyword = true
		}
	}
	if !foundKeyword {
		t.Fatalf("expected the added line to tokenize as real code, not comment continuation, got %+v", h.Lines[1].Tokens)
	}
}

// TestSplitTokensByLineHandlesMultiLineTokens guards the actual mechanism:
// a single token whose Value spans several lines (a CommentMultiline is
// the common case) must be split at each "\n" into separate per-line
// pieces that keep the original token's Type, not dropped or merged into
// the wrong line.
func TestSplitTokensByLineHandlesMultiLineTokens(t *testing.T) {
	tokens := []chroma.Token{
		{Type: chroma.CommentMultiline, Value: "/*\nline two\n*/"},
		{Type: chroma.Text, Value: "\n"},
		{Type: chroma.Keyword, Value: "return"},
	}
	got := splitTokensByLine(tokenIterator(tokens), 4)
	if len(got) != 4 {
		t.Fatalf("expected 4 line buckets, got %d", len(got))
	}
	want := []string{"/*", "line two", "*/", "return"}
	for i, w := range want {
		if len(got[i]) != 1 || got[i][0].Value != w {
			t.Fatalf("line %d: expected a single token %q, got %+v", i, w, got[i])
		}
	}
	if got[0][0].Type != chroma.CommentMultiline || got[2][0].Type != chroma.CommentMultiline {
		t.Fatalf("expected the split comment pieces to keep CommentMultiline, got %+v / %+v", got[0], got[2])
	}
	if got[3][0].Type != chroma.Keyword {
		t.Fatalf("expected the trailing token to keep its own type, got %+v", got[3])
	}
}

// TestHighlightContentFallsBackWhenTokensNil guards backward
// compatibility: highlightContent must still tokenize content itself
// (the old per-line behavior) when tokens is nil — e.g. a Line built
// directly in a test, or any other path that never went through
// flattenFile/applySyntaxTokens.
func TestHighlightContentFallsBackWhenTokensNil(t *testing.T) {
	lexer := pickLexer("a.go")
	out := highlightContent(lexer, syntaxContextFmt, "func main() {}", nil, nil)
	if out == "func main() {}" {
		t.Fatalf("expected the nil-tokens fallback to still syntax-highlight via lexer, got plain text back: %q", out)
	}
}
