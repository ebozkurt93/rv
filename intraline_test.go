package main

import "testing"

func TestComputeIntralineMasksIdenticalLines(t *testing.T) {
	oldMask, newMask := computeIntralineMasks("same line", "same line")
	if oldMask != nil || newMask != nil {
		t.Fatalf("expected no masks for identical lines, got old=%v new=%v", oldMask, newMask)
	}
}

func TestComputeIntralineMasksMarksOnlyTheChangedWord(t *testing.T) {
	oldMask, newMask := computeIntralineMasks("var foo = 1", "var bar = 1")
	if oldMask == nil || newMask == nil {
		t.Fatalf("expected masks for a single-word change")
	}
	// "var " (0-3) untouched, "foo" (4-6) changed, " = 1" untouched.
	for i, want := range []bool{false, false, false, false, true, true, true, false, false, false, false} {
		if oldMask[i] != want {
			t.Fatalf("oldMask[%d] = %v, want %v (mask=%v)", i, oldMask[i], want, oldMask)
		}
	}
	for i, want := range []bool{false, false, false, false, true, true, true, false, false, false, false} {
		if newMask[i] != want {
			t.Fatalf("newMask[%d] = %v, want %v (mask=%v)", i, newMask[i], want, newMask)
		}
	}
}

func TestComputeIntralineMasksSkipsDissimilarLines(t *testing.T) {
	oldMask, newMask := computeIntralineMasks("aaaa bbbb cccc dddd", "wxyz qrst mnop ijkl")
	if oldMask != nil || newMask != nil {
		t.Fatalf("expected no masks for wholly unrelated lines, got old=%v new=%v", oldMask, newMask)
	}
}

func TestApplyIntralineHighlightsPairsRemovedAddedRuns(t *testing.T) {
	old1, new1 := 1, 1
	h := Hunk{Lines: []Line{
		{Kind: LineContext, Content: "ctx", OldLine: &old1, NewLine: &new1},
		{Kind: LineRemoved, Content: "var foo = 1", OldLine: &old1},
		{Kind: LineAdded, Content: "var bar = 1", NewLine: &new1},
	}}
	applyIntralineHighlights(&h)

	if h.Lines[0].Highlight != nil {
		t.Fatalf("context line should never get a highlight mask")
	}
	if h.Lines[1].Highlight == nil || h.Lines[2].Highlight == nil {
		t.Fatalf("expected both paired lines to get highlight masks")
	}
	if !h.Lines[1].Highlight[4] || !h.Lines[2].Highlight[4] {
		t.Fatalf("expected the changed word's position to be marked in both masks")
	}
}

func TestApplyIntralineHighlightsLeavesUnpairedRemainderAlone(t *testing.T) {
	old1, old2, new1 := 1, 2, 1
	h := Hunk{Lines: []Line{
		{Kind: LineRemoved, Content: "foo", OldLine: &old1},
		{Kind: LineRemoved, Content: "bar", OldLine: &old2},
		{Kind: LineAdded, Content: "foo", NewLine: &new1},
	}}
	applyIntralineHighlights(&h)

	if h.Lines[0].Highlight != nil {
		t.Fatalf("expected the paired (identical-content) line to get no highlight, got %v", h.Lines[0].Highlight)
	}
	if h.Lines[1].Highlight != nil {
		t.Fatalf("expected the unpaired removed line to be left alone, got %v", h.Lines[1].Highlight)
	}
	if h.Lines[2].Highlight != nil {
		t.Fatalf("expected the paired (identical-content) added line to get no highlight, got %v", h.Lines[2].Highlight)
	}
}

func TestExpandMaskForTabsRepeatsMaskValueAcrossExpandedSpaces(t *testing.T) {
	mask := []bool{true, false, false}
	got := expandMaskForTabs("\tab", mask)
	want := []bool{true, true, true, true, false, false}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
