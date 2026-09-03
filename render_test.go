package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpOverlayTogglesWithoutChangingFrameHeight(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 5)}, Session{}, nil)
	m.width, m.height = 100, 24

	normalHeight := len(strings.Split(m.View().Content, "\n"))

	mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m2, ok := mm.(model)
	if !ok {
		t.Fatalf("Update did not return a model")
	}
	if m2.mode != modeHelp {
		t.Fatalf("expected modeHelp after '?', got %v", m2.mode)
	}

	helpHeight := len(strings.Split(m2.View().Content, "\n"))
	if normalHeight != helpHeight {
		t.Fatalf("frame height changed opening help: normal=%d help=%d", normalHeight, helpHeight)
	}
}

func TestHelpOverlayClosesOnEscHelpOrQuestionMark(t *testing.T) {
	base := newModel("/repo", nil, Session{}, nil)
	base.mode = modeHelp

	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyEsc},
		{Code: '?', Text: "?"},
	} {
		mm, _ := base.updateHelp(key)
		m := mm.(model)
		if m.mode != modeNormal {
			t.Fatalf("expected %v to close help, mode is still %v", key, m.mode)
		}
	}
}

func TestHelpOverlayIgnoresUnrelatedKeys(t *testing.T) {
	base := newModel("/repo", nil, Session{}, nil)
	base.mode = modeHelp

	mm, _ := base.updateHelp(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m := mm.(model)
	if m.mode != modeHelp {
		t.Fatalf("expected an unrelated key to leave help open, got mode=%v", m.mode)
	}
}

// TestHelpOverlayScrollsPastOneScreen guards the fix for "? help overlay
// needs to scroll" — helpRows() has more entries than fit on a short
// terminal, and before this fix renderHelp truncated via fitBlock instead
// of scrolling, so entries past the visible height were unreachable.
func TestHelpOverlayScrollsPastOneScreen(t *testing.T) {
	m := newModel("/repo", nil, Session{}, nil)
	m.width, m.height = 80, 10 // short enough that helpLines() overflows one screen
	m.mode = modeHelp

	if len(m.helpLines()) <= m.bodyHeight()-borderOverheadH {
		t.Fatalf("test needs a terminal short enough that help overflows one screen")
	}

	mm, _ := m.updateHelp(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m2 := mm.(model)
	if m2.helpScroll != 1 {
		t.Fatalf("expected 'j' to scroll the help overlay down by 1, got helpScroll=%d", m2.helpScroll)
	}

	// Scrolling can't run past the point where the last line is on screen.
	for i := 0; i < 1000; i++ {
		mm, _ = m2.updateHelp(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m2 = mm.(model)
	}
	maxScroll := len(m2.helpLines()) - (m2.bodyHeight() - borderOverheadH)
	if m2.helpScroll != maxScroll {
		t.Fatalf("expected scrolling to clamp at %d, got %d", maxScroll, m2.helpScroll)
	}

	mm, _ = m2.updateHelp(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m3 := mm.(model)
	if m3.helpScroll != maxScroll-1 {
		t.Fatalf("expected 'k' to scroll back up by 1, got helpScroll=%d", m3.helpScroll)
	}
}

// TestMouseWheelScrollsHelpOverlay guards the mouse-wheel counterpart to
// j/k/ctrl+d/ctrl+u — scrolling the help overlay shouldn't require a
// keyboard.
func TestMouseWheelScrollsHelpOverlay(t *testing.T) {
	m := newModel("/repo", nil, Session{}, nil)
	m.width, m.height = 80, 10
	m.mode = modeHelp

	mm, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m2 := mm.(model)
	if m2.helpScroll <= 0 {
		t.Fatalf("expected wheel-down to scroll the help overlay down, got helpScroll=%d", m2.helpScroll)
	}

	mm, _ = m2.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m3 := mm.(model)
	if m3.helpScroll >= m2.helpScroll {
		t.Fatalf("expected wheel-up to scroll back up, got helpScroll=%d (was %d)", m3.helpScroll, m2.helpScroll)
	}
}

// TestCommentEditorHasNoBorder guards the fix for "comment editor shouldn't
// be a bordered box" — it should read as an inline row matching
// renderComment/renderReply's own "  │..." indent convention, not a boxed
// dialog with its own border characters.
func TestCommentEditorHasNoBorder(t *testing.T) {
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{}, nil)
	m.width, m.height = 100, 24
	m.mode = modeComment
	m.input = "looks good"

	out := m.renderCommentEditor()
	// Corner characters are unambiguous evidence of an actual bordered box
	// — unlike panelBorder.Left, which happens to be the same "│" glyph the
	// comment thread's own connector column deliberately reuses (see
	// renderComment), so checking for that would false-positive now.
	if strings.Contains(out, panelBorder.TopLeft) {
		t.Fatalf("expected no border characters in the comment editor, got %q", out)
	}
	if !strings.Contains(out, "│") {
		t.Fatalf("expected the same '│' indent convention as renderComment/renderReply, got %q", out)
	}
	if !strings.Contains(out, "looks good") {
		t.Fatalf("expected the typed input to appear in the rendered editor, got %q", out)
	}
}

// commentConnectorColumn returns the cell column of the first "│" in s, or
// -1 if there isn't one — used to check the connector stays in the same
// column across a comment's continuation lines and its replies, tree-style,
// rather than comparing byte offsets (the prefix has multi-byte runes, so a
// byte offset would overcount relative to actual terminal columns).
func commentConnectorColumn(s string) int {
	idx := strings.IndexAny(s, "│├└")
	if idx < 0 {
		return -1
	}
	return ansi.StringWidth(s[:idx])
}

// TestCommentBodyLinesDoesNotPadShortLinesToWidestSibling guards a real
// bug: lipgloss's Style.Render() right-pads every line of multi-line input
// out to the width of the WIDEST line in that same call. renderComment/
// renderReply used to join a whole multi-paragraph body into one string
// and style it with a single Render() call — so a short paragraph, or the
// blank separator between two paragraphs, came back carrying a phantom
// width matching the longest paragraph in the body. Left in place, that
// made fitLine truncate a genuinely short/blank line with a spurious "…"
// in non-wrap mode, and made wrapLineIndented (which measures width from
// this output) wrap far more of a line than it actually had content for.
func TestCommentBodyLinesDoesNotPadShortLinesToWidestSibling(t *testing.T) {
	body := "short\n\nsecond paragraph is much much much much much much longer than the first one by a lot"
	out := renderReply(Reply{Author: "agent", Body: body}, false, true)
	rows := strings.Split(out, "\n")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (paragraph, blank separator, paragraph), got %d: %v", len(rows), rows)
	}
	shortWidth := ansi.StringWidth(rows[0])
	longWidth := ansi.StringWidth(rows[2])
	if shortWidth >= longWidth {
		t.Fatalf("expected the short first line's rendered width (%d) to stay well under the long paragraph's (%d), got padded to match it", shortWidth, longWidth)
	}
	blankWidth := ansi.StringWidth(rows[1])
	if blankWidth >= longWidth {
		t.Fatalf("expected the blank separator's rendered width (%d) to stay short, got padded toward the long paragraph's (%d)", blankWidth, longWidth)
	}
}

// TestCommentContinuationLineKeepsConnectorColumn guards the fix for "so
// comment indicator is still not aligned... I want something similar to
// what tree command has" — a multi-line comment's continuation rows (and
// its replies) must keep "│" in the same column as the first line's, like
// `tree` keeps "│" in a fixed column down consecutive rows, instead of the
// connector appearing to end after the first line.
func TestCommentContinuationLineKeepsConnectorColumn(t *testing.T) {
	// Reply bodies here are deliberately single-line — a reply's own
	// wrapped continuation doesn't carry the connector forward (there's
	// nothing branching further off a leaf), so only the comment's own
	// continuation and its replies' first lines are expected to align.
	c := Comment{Author: "user", Body: "first line\nsecond line",
		Replies: []Reply{{Author: "agent", Body: "a reply"}, {Author: "user", Body: "another reply"}}}

	rows := strings.Split(renderComment(c, false), "\n")
	for i, r := range c.Replies {
		last := i == len(c.Replies)-1
		rows = append(rows, strings.Split(renderReply(r, false, last), "\n")...)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 rendered rows, got %d: %v", len(rows), rows)
	}

	want := commentConnectorColumn(rows[0])
	if want < 0 {
		t.Fatalf("expected the first row to contain a '│' connector: %q", rows[0])
	}
	for _, row := range rows[1:] {
		if got := commentConnectorColumn(row); got != want {
			t.Fatalf("expected '│' at column %d on every row, got %d in %q", want, got, row)
		}
	}
}

// TestWrappedCommentReplyStaysAlignedUnderConnector is
// TestCommentContinuationLineKeepsConnectorColumn's counterpart for a
// SINGLE logical line that's too long to fit and has to word-wrap — its
// physical continuation rows must also stay aligned under the connector
// column instead of falling back to column 0 (the actual bug reported:
// wrapping a long reply visibly detached its overflow text from the
// thread it belonged to). Uses a NON-last reply (a second, short reply
// follows it) — a last reply's own wrap continuation is deliberately blank
// instead (see TestLastReplyBlanksConnectorThroughItsOwnParagraphs).
func TestWrappedCommentReplyStaysAlignedUnderConnector(t *testing.T) {
	withTempHome(t)
	n := 1
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineContext, Content: "x", OldLine: &n, NewLine: &n},
	}}}}
	longBody := "This is a deliberately very long single reply line meant to force word-wrapping across several physical rows once it exceeds the diff pane's available width."
	sess := Session{Comments: []Comment{
		{ID: "c1", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: "short",
			Replies: []Reply{
				{ID: "r1", Body: longBody, Author: "agent"},
				{ID: "r2", Body: "ok", Author: "user"},
			}},
	}}
	m := newModel("/repo", []FileDiff{fd}, sess, nil)
	m.wrapLines = true

	lines, _, _, mainLine := m.buildDiffLinesDetailed(40)
	var replyRows []string
	inReply := false
	for i, l := range lines {
		stripped := ansi.Strip(l)
		if strings.Contains(stripped, "agent:") {
			inReply = true
		} else if mainLine[i] || strings.TrimSpace(stripped) == "" {
			inReply = false
		}
		if inReply {
			replyRows = append(replyRows, stripped)
		}
	}
	if len(replyRows) < 2 {
		t.Fatalf("expected the long reply to wrap into multiple rows at width 40, got %d: %v", len(replyRows), replyRows)
	}
	want := commentConnectorColumn(replyRows[0])
	if want < 0 {
		t.Fatalf("expected the reply's first row to contain a connector: %q", replyRows[0])
	}
	for _, row := range replyRows[1:] {
		// A wrapped continuation must repeat the "│" bar at the same
		// column as the first row's own connector (see commentBarPad) —
		// this is the actual bug reported live: a reply whose first line
		// wrapped due to terminal width lost its connector on every
		// continuation row, making the thread look visibly cut/detached
		// even though wrapLineIndented kept the indentation right.
		got := commentConnectorColumn(row)
		if got != want {
			t.Fatalf("expected wrapped continuation %q to keep the connector bar at column %d, got %d", row, want, got)
		}
	}
}

// TestReplyBranchesLikeTree guards the fix for the "└" appearing on every
// reply regardless of position — only the last reply should cap the thread
// with "└─"; every earlier one gets "├─" so the connector keeps running to
// whatever follows, exactly like `tree` branches a directory listing.
func TestReplyBranchesLikeTree(t *testing.T) {
	first := renderReply(Reply{Author: "agent", Body: "a"}, false, false)
	last := renderReply(Reply{Author: "user", Body: "b"}, false, true)
	if !strings.Contains(first, "├─") {
		t.Fatalf("expected a non-last reply to branch with '├─', got %q", first)
	}
	if !strings.Contains(last, "└─") {
		t.Fatalf("expected the last reply to cap the thread with '└─', got %q", last)
	}
}

// TestLastReplyBlanksConnectorThroughItsOwnParagraphs guards the actual bug
// reported live ("you see that it does not connect"): "└" terminates its
// own vertical stroke by turning the corner — it never continues downward
// in any font — so a "│" placed directly beneath it always reads as
// visually disconnected from the corner above it, not as a continuation of
// it, regardless of color or column alignment being otherwise correct. An
// earlier version deliberately kept the bar running through the last
// reply's own later paragraphs (worried they'd otherwise look orphaned),
// but a wrong-looking connector is worse than a quieter one — the shared
// indentation alone still visually groups the paragraphs under the reply
// that owns them.
func TestLastReplyBlanksConnectorThroughItsOwnParagraphs(t *testing.T) {
	body := "first paragraph\n\nsecond paragraph"
	out := renderReply(Reply{Author: "agent", Body: body}, false, true)
	rows := strings.Split(out, "\n")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (paragraph, blank separator, paragraph), got %d: %v", len(rows), rows)
	}
	if got := commentConnectorColumn(rows[0]); got < 0 {
		t.Fatalf("expected the first row to contain a connector (the '└' branch itself): %q", rows[0])
	}
	for _, row := range rows[1:] {
		if got := commentConnectorColumn(row); got >= 0 {
			t.Fatalf("expected no connector on the last reply's own later paragraphs (blank instead, since '└' doesn't continue downward), got one at column %d in %q", got, row)
		}
	}
}

// TestNonLastReplyKeepsConnectorThroughItsOwnParagraphs is the sibling
// guard: a non-last reply's branch is "├", which DOES have a stroke
// continuing downward (something else really does follow at this level in
// the thread) — so its own later paragraphs correctly keep the bar.
func TestNonLastReplyKeepsConnectorThroughItsOwnParagraphs(t *testing.T) {
	body := "first paragraph\n\nsecond paragraph"
	out := renderReply(Reply{Author: "agent", Body: body}, false, false)
	rows := strings.Split(out, "\n")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (paragraph, blank separator, paragraph), got %d: %v", len(rows), rows)
	}
	want := commentConnectorColumn(rows[0])
	if want < 0 {
		t.Fatalf("expected the first row to contain a connector: %q", rows[0])
	}
	for _, row := range rows[1:] {
		if got := commentConnectorColumn(row); got != want {
			t.Fatalf("expected the connector to keep running through a non-last reply's own paragraphs at column %d, got %d in %q", want, got, row)
		}
	}
}

// TestComposingReplyKeepsConnectorRunning guards the fix for a visible
// "gap": composing a new reply renders below the existing ones, so the
// previously-last reply must flip from "└─" to "├─" (something now follows
// it) and the in-progress editor row must itself become the new "└"
// terminal branch — otherwise the thread looks like it already ended right
// before more of it appears below. Exercises the real buildDiffLinesDetailed
// path, not a re-implementation of its branch logic.
func TestComposingReplyKeepsConnectorRunning(t *testing.T) {
	n := 1
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Author: "user", Body: "original",
			Replies: []Reply{{ID: "r_1", Author: "agent", Body: "first reply"}}},
	}}, nil)
	m.width = 100
	m.lineIndex = 1 // the commented line
	m.mode = modeComment
	m.replyingToCommentID = "c_1"
	m.input = "another reply"

	lines, _, _, _ := m.buildDiffLinesDetailed(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "├─") {
		t.Fatalf("expected the previously-last reply to branch with '├─' while a new one is being composed, got %q", joined)
	}
	if !strings.Contains(joined, "└─ ↩") {
		t.Fatalf("expected the in-progress reply row to use the terminal branch '└', got %q", joined)
	}
}

func TestReplyStyleReflectsThreadResolution(t *testing.T) {
	reply := Reply{Author: "agent", Body: "on it"}

	unresolved := renderReply(reply, false, true)
	resolved := renderReply(reply, true, true)
	if unresolved == resolved {
		t.Fatalf("expected resolved vs unresolved threads to style a reply differently")
	}
}

// TestResolvedThreadCollapsesByDefault guards the actual feature: a
// resolved comment renders as one collapsed summary line (not the full
// comment + replies), until Enter (Confirm) expands it. An unresolved one
// always shows in full regardless.
func TestResolvedThreadCollapsesByDefault(t *testing.T) {
	n := 1
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineContext, Content: "x", OldLine: &n, NewLine: &n},
	}}}}
	sess := Session{Comments: []Comment{
		{ID: "c1", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: "fix this", Resolved: true,
			Replies: []Reply{{ID: "r1", Body: "done", Author: "agent"}}},
	}}
	m := newModel("/repo", []FileDiff{fd}, sess, nil)
	m.lineIndex = 1

	lines, _, _, _ := m.buildDiffLinesDetailed(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "resolved · 1 reply") {
		t.Fatalf("expected a collapsed one-line summary, got %q", joined)
	}
	if strings.Contains(joined, "done") {
		t.Fatalf("expected the reply body to be hidden while collapsed, got %q", joined)
	}

	m.toggleCommentsExpandedUnderCursor()
	expanded, _, _, _ := m.buildDiffLinesDetailed(80)
	expandedJoined := strings.Join(expanded, "\n")
	if !strings.Contains(expandedJoined, "fix this") || !strings.Contains(expandedJoined, "done") {
		t.Fatalf("expected the full thread after Confirm toggles it open, got %q", expandedJoined)
	}
}

// TestUnresolvedCommentNeverCollapses guards the other half: an unresolved
// comment has nothing to hide and always renders in full.
func TestUnresolvedCommentNeverCollapses(t *testing.T) {
	n := 1
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineContext, Content: "x", OldLine: &n, NewLine: &n},
	}}}}
	sess := Session{Comments: []Comment{
		{ID: "c1", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: "still open"},
	}}
	m := newModel("/repo", []FileDiff{fd}, sess, nil)

	lines, _, _, _ := m.buildDiffLinesDetailed(80)
	if !strings.Contains(strings.Join(lines, "\n"), "still open") {
		t.Fatalf("expected an unresolved comment to render in full, got %q", lines)
	}
}

// TestToggleCommentsExpandedTogglesMultipleResolvedThreadsTogether guards
// the multi-thread-on-one-line design: a line can accumulate several old
// resolved discussions (e.g. from before the code around it changed) —
// Confirm should expand/collapse all of them together in one press, not
// require toggling each individually, and must leave an unresolved thread
// on the same line untouched (always expanded).
func TestToggleCommentsExpandedTogglesMultipleResolvedThreadsTogether(t *testing.T) {
	n := 1
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineContext, Content: "x", OldLine: &n, NewLine: &n},
	}}}}
	sess := Session{Comments: []Comment{
		{ID: "old1", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: "old q1", Resolved: true},
		{ID: "old2", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: "old q2", Resolved: true},
		{ID: "fresh", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: "still open q"},
	}}
	m := newModel("/repo", []FileDiff{fd}, sess, nil)
	m.lineIndex = 1

	before := strings.Join(func() []string { l, _, _, _ := m.buildDiffLinesDetailed(80); return l }(), "\n")
	if got := strings.Count(before, "▸"); got != 2 {
		t.Fatalf("expected both resolved threads collapsed (2 '▸' markers), got %d in %q", got, before)
	}
	if !strings.Contains(before, "still open q") {
		t.Fatalf("expected the unresolved thread to always show in full, got %q", before)
	}

	m.toggleCommentsExpandedUnderCursor()
	after := strings.Join(func() []string { l, _, _, _ := m.buildDiffLinesDetailed(80); return l }(), "\n")
	if strings.Contains(after, "▸") {
		t.Fatalf("expected Confirm to expand BOTH resolved threads together (no '▸' left), got %q", after)
	}
	if !strings.Contains(after, "old q1") || !strings.Contains(after, "old q2") {
		t.Fatalf("expected the full body text of both threads after expanding, got %q", after)
	}
}

// TestClampAdditionalScrollPagesPastCenteredWindow guards the fix for a
// real gap: a comment thread taller than the viewport used to be
// unreachable past clampScroll's height/2 window below the cursor's own
// row, with no way to see the rest of it — j/k only moves between rows,
// and moving off the current row re-centers on a different row's block
// entirely. clampAdditionalScroll must let a positive offset push the
// window further down (revealing more of the block below), clamped to the
// same [0, total-height] bounds clampScroll itself respects.
func TestClampAdditionalScrollPagesPastCenteredWindow(t *testing.T) {
	base := clampScroll(5, 100, 20) // cursor at row 5, centered
	if got := clampAdditionalScroll(base, 0, 100, 20); got != base {
		t.Fatalf("expected zero offset to leave the base scroll unchanged, got %d want %d", got, base)
	}
	if got := clampAdditionalScroll(base, 10, 100, 20); got != base+10 {
		t.Fatalf("expected a positive offset to scroll further down, got %d want %d", got, base+10)
	}
	if got := clampAdditionalScroll(base, 1000, 100, 20); got != 80 {
		t.Fatalf("expected the offset to clamp at total-height (80), got %d", got)
	}
	if got := clampAdditionalScroll(base, -1000, 100, 20); got != 0 {
		t.Fatalf("expected the offset to clamp at 0, got %d", got)
	}
}

// TestScrollDownRevealsMoreOfATallCommentThread exercises the real key
// handling path (updateNormal) end to end: a reply long enough to wrap
// past the viewport's height must have more of its later lines revealed
// by repeated ScrollDown presses, without moving m.lineIndex off the row
// it started on.
func TestScrollDownRevealsMoreOfATallCommentThread(t *testing.T) {
	withTempHome(t)
	n := 1
	fd := FileDiff{Path: "a.go", Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: []Line{
		{Kind: LineContext, Content: "x", OldLine: &n, NewLine: &n},
	}}}}
	var paragraphs []string
	for i := 0; i < 40; i++ {
		paragraphs = append(paragraphs, "paragraph number "+strings.Repeat("z", i%3)+" "+string(rune('a'+i%26)))
	}
	longBody := strings.Join(paragraphs, "\n\n")
	sess := Session{Comments: []Comment{
		{ID: "c1", File: "a.go", NewLine: &n, LineContent: "x", Author: "user", Body: longBody},
	}}
	m := newModel("/repo", []FileDiff{fd}, sess, nil)
	m.width, m.height = 80, 15
	startLine := m.lineIndex

	firstFrame := m.renderDiff()

	mm, _ := m.updateNormal(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl, Text: ""})
	m2 := mm.(model)
	for i := 0; i < 20; i++ {
		mm2, _ := m2.updateNormal(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl, Text: ""})
		m2 = mm2.(model)
	}
	if m2.lineIndex != startLine {
		t.Fatalf("expected ScrollDown to leave the cursor row unchanged, got %d want %d", m2.lineIndex, startLine)
	}
	if m2.diffScroll == 0 {
		t.Fatalf("expected repeated ScrollDown presses to accumulate a manual scroll offset")
	}
	scrolledFrame := m2.renderDiff()
	if scrolledFrame == firstFrame {
		t.Fatalf("expected the rendered frame to change after scrolling down through a tall thread")
	}

	// Moving the cursor resets the manual scroll — it only means anything
	// relative to the row it was set on.
	m2.moveCursor(0)
	if m2.diffScroll != 0 {
		t.Fatalf("expected moveCursor to reset diffScroll back to 0, got %d", m2.diffScroll)
	}
}
