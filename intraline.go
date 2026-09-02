package main

import (
	"regexp"
	"unicode"
	"unicode/utf8"
)

// intralineTokenPattern splits line content into runs of "word" characters
// (letters/digits/underscore, Unicode-aware), runs of whitespace, or single
// other runes (punctuation/operators) — one token per visually distinct
// unit, so a rename highlights the whole identifier rather than each
// changed letter, and a single changed operator doesn't drag its neighbors
// in with it.
var intralineTokenPattern = regexp.MustCompile(`[\p{L}\p{N}_]+|\s+|.`)

// minIntralineSimilarity is the floor (fraction of the longer token list
// that matches) below which a removed/added pair is treated as an
// unrelated replacement rather than an edit — at that point every token
// would end up "changed" anyway, so marking the whole line adds visual
// noise without adding information the existing whole-line +/- tint
// doesn't already convey.
const minIntralineSimilarity = 0.2

// applyIntralineHighlights walks h's lines and fills in Line.Highlight for
// each removed/added line that's part of a same-size-run pairing — reusing
// the exact pairing zipChangeBlock uses for the split view, so a line's
// intraline highlight always lines up with whichever line it's shown
// beside there.
func applyIntralineHighlights(h *Hunk) {
	i := 0
	for i < len(h.Lines) {
		if h.Lines[i].Kind == LineContext {
			i++
			continue
		}
		start := i
		for i < len(h.Lines) && h.Lines[i].Kind != LineContext {
			i++
		}
		highlightChangeBlock(h.Lines[start:i])
	}
}

// highlightChangeBlock mirrors zipChangeBlock's pairing (see splitpair.go):
// removed and added lines within the block are separated out preserving
// their own relative order, then zipped index-by-index. Only the paired
// prefix gets intraline highlights; either side's unpaired remainder is
// left alone.
func highlightChangeBlock(block []Line) {
	var removed, added []int // indices into block
	for idx, l := range block {
		switch l.Kind {
		case LineRemoved:
			removed = append(removed, idx)
		case LineAdded:
			added = append(added, idx)
		}
	}
	n := len(removed)
	if len(added) < n {
		n = len(added)
	}
	for k := 0; k < n; k++ {
		oldLine := &block[removed[k]]
		newLine := &block[added[k]]
		oldMask, newMask := computeIntralineMasks(oldLine.Content, newLine.Content)
		oldLine.Highlight = oldMask
		newLine.Highlight = newMask
	}
}

// computeIntralineMasks returns per-rune "this rune changed" masks for a
// removed/added line pair, or nil, nil if the lines are identical or too
// dissimilar to bother (see minIntralineSimilarity).
func computeIntralineMasks(oldContent, newContent string) (oldMask, newMask []bool) {
	if oldContent == newContent {
		return nil, nil
	}
	oldTok := intralineTokenPattern.FindAllString(oldContent, -1)
	newTok := intralineTokenPattern.FindAllString(newContent, -1)

	matchedOld, matchedNew := diffTokens(oldTok, newTok)

	// Similarity is judged on non-whitespace tokens only — two otherwise
	// unrelated lines sharing ordinary spacing shouldn't read as "related"
	// just because their whitespace runs happen to line up.
	common, maxLen := 0, 0
	for i, tok := range oldTok {
		if isWhitespaceToken(tok) {
			continue
		}
		maxLen++
		if matchedOld[i] {
			common++
		}
	}
	newSignificant := 0
	for _, tok := range newTok {
		if !isWhitespaceToken(tok) {
			newSignificant++
		}
	}
	if newSignificant > maxLen {
		maxLen = newSignificant
	}
	if maxLen == 0 || float64(common)/float64(maxLen) < minIntralineSimilarity {
		return nil, nil
	}

	bridgeWhitespaceGaps(oldTok, matchedOld)
	bridgeWhitespaceGaps(newTok, matchedNew)

	return expandTokenMask(oldContent, oldTok, matchedOld), expandTokenMask(newContent, newTok, matchedNew)
}

// bridgeWhitespaceGaps folds a single matched whitespace token into the
// surrounding highlight wherever it sits directly between two unmatched
// (changed) tokens — e.g. two consecutive rewritten words separated by one
// space. Without this, a run of several changed words renders as a series
// of highlighted words with a one-character gap of the plain tint between
// each: visually indistinguishable from noise since the plain and strong
// tints share the same hue, rather than reading as one changed clause.
// Mutates matched in place.
func bridgeWhitespaceGaps(tokens []string, matched []bool) {
	for i, tok := range tokens {
		if !matched[i] || !isWhitespaceToken(tok) {
			continue
		}
		if i > 0 && !matched[i-1] && i < len(tokens)-1 && !matched[i+1] {
			matched[i] = false
		}
	}
}

// isWhitespaceToken reports whether tok is a run of whitespace — such a
// token always matches intralineTokenPattern's `\s+` branch entirely (it
// can't have mixed whitespace/non-whitespace runes), so checking the first
// rune is enough.
func isWhitespaceToken(tok string) bool {
	r, _ := utf8.DecodeRuneInString(tok)
	return unicode.IsSpace(r)
}

// diffTokens finds a longest common subsequence between a and b (dynamic
// programming over token identity, not similarity — fine at line-length
// token counts) and returns, for each side, which tokens are part of it.
// An unmatched token is one the LCS backtrack skipped over — i.e. it was
// removed (old side) or added (new side).
func diffTokens(a, b []string) (matchedA, matchedB []bool) {
	n, m := len(a), len(b)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	matchedA = make([]bool, n)
	matchedB = make([]bool, m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			matchedA[i] = true
			matchedB[j] = true
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return matchedA, matchedB
}

// expandTokenMask turns a per-token matched/unmatched verdict into a
// per-rune mask over content (true = unmatched = part of the intraline
// highlight), since rendering operates on rune positions within the line,
// not tokens.
func expandTokenMask(content string, tokens []string, matched []bool) []bool {
	mask := make([]bool, utf8.RuneCountInString(content))
	pos := 0
	for ti, tok := range tokens {
		tlen := utf8.RuneCountInString(tok)
		if !matched[ti] {
			for k := pos; k < pos+tlen && k < len(mask); k++ {
				mask[k] = true
			}
		}
		pos += tlen
	}
	return mask
}

// expandMaskForTabs re-expresses mask (indexed over content's runes) as a
// mask indexed over content with every tab rune replaced by four spaces —
// matching renderLine's own tab expansion of visibleContent, so the two
// stay aligned rune-for-rune.
func expandMaskForTabs(content string, mask []bool) []bool {
	if mask == nil {
		return nil
	}
	out := make([]bool, 0, len(mask))
	i := 0
	for _, r := range content {
		v := i < len(mask) && mask[i]
		if r == '\t' {
			out = append(out, v, v, v, v)
		} else {
			out = append(out, v)
		}
		i++
	}
	return out
}
