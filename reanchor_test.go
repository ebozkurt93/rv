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

	idx, ok := commentAnchorRow(fr, c)
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

	idx, ok := commentAnchorRow(fr, c)
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

	if _, ok := commentAnchorRow(fr, c); ok {
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

	if _, ok := commentAnchorRow(fr, c); ok {
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

	idx, ok := commentAnchorRow(fr, c)
	if !ok || fr.rows[idx].line.Content != "func b()" {
		t.Fatalf("expected legacy pure line-number match, got idx=%d ok=%v", idx, ok)
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
