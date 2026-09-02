package main

import "testing"

// fileDiffWithContentLines builds a single-hunk file whose N context lines
// have distinct, caller-provided content (fileDiffWithLines' lines are all
// identical "x", which can't exercise content-based matching).
func fileDiffWithContentLines(path string, content ...string) FileDiff {
	var lines []Line
	for i, c := range content {
		n := i + 1
		lines = append(lines, Line{Kind: LineContext, Content: c, OldLine: &n, NewLine: &n})
	}
	return FileDiff{Path: path, Status: FileModified, Hunks: []Hunk{{Header: "h", Lines: lines}}}
}

func TestCommentAnchorRowStaysPutWhenContentMatches(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "func a()", "func b()", "func c()"))
	n := 2
	c := Comment{File: "a.go", NewLine: &n, LineContent: "func b()"}

	idx, ok, _ := commentAnchorRow(fr, c)
	if !ok || fr.rows[idx].line.Content != "func b()" {
		t.Fatalf("expected anchor at the matching row, got idx=%d ok=%v", idx, ok)
	}
}

// TestCommentAnchorRowReanchorsWhenLineMoved guards the actual fix: if the
// line number no longer holds the comment's original content (something
// shifted above it) but that exact content exists at exactly one other row,
// the comment re-anchors there instead of silently detaching or attaching
// to whatever unrelated line now sits at the old number.
func TestCommentAnchorRowReanchorsWhenLineMoved(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "func a()", "func new()", "func b()"))
	n := 2 // originally on row for "func b()" before "func new()" was inserted above it
	c := Comment{File: "a.go", NewLine: &n, LineContent: "func b()"}

	idx, ok, _ := commentAnchorRow(fr, c)
	if !ok || fr.rows[idx].line.Content != "func b()" {
		t.Fatalf("expected re-anchor to the row with matching content, got idx=%d ok=%v", idx, ok)
	}
}

// TestCommentAnchorRowOrphansOnAmbiguousContent guards the "worse" failure
// mode this replaces: reattaching to an unrelated line that happens to
// share content is only safe when the match is unique. Duplicate content
// must leave the comment unanchored (orphaned) rather than guessing.
func TestCommentAnchorRowOrphansOnAmbiguousContent(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "}", "func b()", "}"))
	n := 99 // no longer matches any row by number
	c := Comment{File: "a.go", NewLine: &n, LineContent: "}"}

	if _, ok, _ := commentAnchorRow(fr, c); ok {
		t.Fatalf("expected an ambiguous (2-way) content match to orphan the comment, not guess")
	}
}

// TestCommentAnchorRowOrphansWhenContentGone guards the case where the
// commented line was simply deleted and doesn't exist anywhere else in the
// file either.
func TestCommentAnchorRowOrphansWhenContentGone(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "func a()", "func b()"))
	n := 99
	c := Comment{File: "a.go", NewLine: &n, LineContent: "func deleted()"}

	if _, ok, _ := commentAnchorRow(fr, c); ok {
		t.Fatalf("expected no match anywhere in the file to orphan the comment")
	}
}

// TestCommentAnchorRowFallsBackToLineNumberWithoutRecordedContent guards
// backward compatibility: a comment created before LineContent existed
// (empty string) must behave exactly like the old pure line-number match,
// regardless of what the line's content actually is now.
func TestCommentAnchorRowFallsBackToLineNumberWithoutRecordedContent(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "func a()", "func b()"))
	n := 2
	c := Comment{File: "a.go", NewLine: &n} // LineContent left empty

	idx, ok, _ := commentAnchorRow(fr, c)
	if !ok || fr.rows[idx].line.Content != "func b()" {
		t.Fatalf("expected legacy pure line-number match, got idx=%d ok=%v", idx, ok)
	}
}

// TestCommentAnchorRowFallsBackToStaleLineNumberWhenContentChanged guards
// the actual bug this fixes: an agent editing the exact line a comment was
// left on (the common "leave feedback, agent fixes it" flow) changes that
// line's content, so it no longer matches anywhere in the file — content
// matching alone would orphan (hide) the comment entirely. It should
// instead re-anchor to the original line number, on whatever content is
// there now, flagged stale.
func TestCommentAnchorRowFallsBackToStaleLineNumberWhenContentChanged(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "func a()", "func b()", "func c()"))
	n := 2 // "func b()" at creation time; since edited to "func b2()"
	c := Comment{File: "a.go", NewLine: &n, LineContent: "func b()"}
	fr.rows[2].line.Content = "func b2()" // rows[0] is the hunk header

	idx, ok, stale := commentAnchorRow(fr, c)
	if !ok || !stale {
		t.Fatalf("expected a stale fallback anchor, got idx=%d ok=%v stale=%v", idx, ok, stale)
	}
	if fr.rows[idx].line.Content != "func b2()" {
		t.Fatalf("expected the fallback to land on the row at the original line number, got %q", fr.rows[idx].line.Content)
	}
}

// TestCommentAnchorRowFreshMatchIsNotStale is the sibling guard: an exact
// or unique-content match must NOT be flagged stale — only the last-resort
// line-number-only fallback should be.
func TestCommentAnchorRowFreshMatchIsNotStale(t *testing.T) {
	fr := flattenFile(fileDiffWithContentLines("a.go", "func a()", "func b()", "func c()"))
	n := 2
	c := Comment{File: "a.go", NewLine: &n, LineContent: "func b()"}

	_, ok, stale := commentAnchorRow(fr, c)
	if !ok || stale {
		t.Fatalf("expected a fresh, non-stale match, got ok=%v stale=%v", ok, stale)
	}
}

// TestCommentsForCurrentLineFreshExcludesStale guards the fix for adding a
// new comment on a line whose only existing comment there is stale: the
// stale one is left on code that no longer exists, so it must not be
// treated as "the" comment on this row for AddComment's edit-in-place
// decision (see commentActionForCurrentLine) — that would silently attach
// new text to a thread about different code.
func TestCommentsForCurrentLineFreshExcludesStale(t *testing.T) {
	withTempHome(t)
	fd := fileDiffWithContentLines("a.go", "func a()", "func b()", "func c()")
	n := 2
	m := newModel("/repo", []FileDiff{fd}, Session{Comments: []Comment{
		{ID: "c1", File: "a.go", NewLine: &n, LineContent: "func b()", Author: "user", Body: "why?"},
	}}, nil)
	m.files[0].rows[2].line.Content = "func b2()" // agent edited the commented line; rows[0] is the hunk header
	m.lineIndex = 2

	if got := m.commentsForCurrentLine(); len(got) != 1 {
		t.Fatalf("expected the stale comment to still render at its row, got %d", len(got))
	}
	if got := m.commentsForCurrentLineFresh(); len(got) != 0 {
		t.Fatalf("expected no fresh comments on the now-different line, got %d", len(got))
	}

	comment, editing, editingReplyID, replying := m.commentActionForCurrentLine()
	if editing || replying || editingReplyID != "" || comment.ID != "" {
		t.Fatalf("expected AddComment to start a brand new thread, not edit the stale one: comment=%+v editing=%v replying=%v",
			comment, editing, replying)
	}
}

// TestCommentAnchorRowSplitFallsBackToStaleLineNumber is
// TestCommentAnchorRowFallsBackToStaleLineNumberWhenContentChanged's
// split-view counterpart.
func TestCommentAnchorRowSplitFallsBackToStaleLineNumber(t *testing.T) {
	fd := fileDiffWithContentLines("a.go", "func a()", "func b()", "func c()")
	fr := flattenFile(fd)
	n := 2
	c := Comment{File: "a.go", NewLine: &n, LineContent: "func b()"}
	// Context rows' left/right point at the same underlying Line, so
	// mutating through either pointer changes what both sides see —
	// exactly what "the line's content changed" means here.
	for i := range fr.splitRows {
		if fr.splitRows[i].right != nil && fr.splitRows[i].right.Content == "func b()" {
			fr.splitRows[i].right.Content = "func b2()"
		}
	}

	idx, ok, stale := commentAnchorRowSplit(fr, c)
	if !ok || !stale {
		t.Fatalf("expected a stale fallback anchor, got idx=%d ok=%v stale=%v", idx, ok, stale)
	}
}

func TestOrphanedCommentCountReflectsUnanchoredComments(t *testing.T) {
	withTempHome(t)
	n1, n2 := 1, 99
	fd := fileDiffWithContentLines("a.go", "func a()", "func b()")
	m := newModel("/repo", []FileDiff{fd}, Session{Comments: []Comment{
		{File: "a.go", NewLine: &n1, LineContent: "func a()"},       // anchored fine
		{File: "a.go", NewLine: &n2, LineContent: "func deleted()"}, // nowhere to anchor
	}}, nil)

	if got := m.orphanedCommentCount(); got != 1 {
		t.Fatalf("expected 1 orphaned comment, got %d", got)
	}
}

// TestAddCommentUnderCursorRecordsLineContent guards that new comments
// actually get the anchoring data commentAnchorRow depends on.
func TestAddCommentUnderCursorRecordsLineContent(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithContentLines("a.go", "func a()", "func b()")}, Session{RepoRoot: "/repo"}, nil)
	m.lineIndex = 2 // "func b()"
	m.mode = modeComment
	m.input = "why?"

	m.addCommentUnderCursor()
	if len(m.session.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(m.session.Comments))
	}
	if got := m.session.Comments[0].LineContent; got != "func b()" {
		t.Fatalf("expected LineContent recorded from the cursor's line, got %q", got)
	}
}
