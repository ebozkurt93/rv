package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestAddCommentOnFreshLineStartsBlank guards the baseline: a line with no
// comment of the user's yet gets a blank editor, not editing/replying.
func TestAddCommentOnFreshLineStartsBlank(t *testing.T) {
	withTempHome(t)
	m := newModel("/repo", []FileDiff{fileDiffWithLines("a.go", 3)}, Session{RepoRoot: "/repo"}, nil)
	m.lineIndex = 1

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := mm.(model)
	if m2.mode != modeComment || m2.input != "" || m2.editingCommentID != "" || m2.replyingToCommentID != "" {
		t.Fatalf("expected a blank new-comment editor, got mode=%v input=%q editing=%q replying=%q",
			m2.mode, m2.input, m2.editingCommentID, m2.replyingToCommentID)
	}
}

// TestAddCommentOnOwnUnrepliedCommentEditsInPlace guards the fix for "if I
// do c when I'm next to an existing comment, I should be editing it again,
// not adding a new comment there" — pressing AddComment on a line with the
// user's own comment (nothing has replied to it yet) pre-fills the editor
// and updates that comment's body on submit, instead of creating a
// duplicate.
func TestAddCommentOnOwnUnrepliedCommentEditsInPlace(t *testing.T) {
	withTempHome(t)
	n := 1
	repo := "/repo"
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Body: "original", Author: "user"},
	}}))
	session, err := loadSession(repo)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, session, nil)
	m.lineIndex = 1

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := mm.(model)
	if m2.mode != modeComment || m2.input != "original" || m2.editingCommentID != "c_1" {
		t.Fatalf("expected the editor pre-filled for in-place editing, got mode=%v input=%q editing=%q",
			m2.mode, m2.input, m2.editingCommentID)
	}

	m2.input = "edited"
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := mm.(model)
	if len(m3.session.Comments) != 1 {
		t.Fatalf("expected editing in place, not a new comment — got %d comments", len(m3.session.Comments))
	}
	if got := m3.session.Comments[0].Body; got != "edited" {
		t.Fatalf("expected the comment's body updated to %q, got %q", "edited", got)
	}
}

// TestAddCommentOnRepliedThreadAddsReply guards the follow-up fix: once
// something has replied to the user's comment, AddComment must not rewrite
// its body (that would silently change what the reply was responding to)
// — it should append a new reply to the thread instead.
func TestAddCommentOnRepliedThreadAddsReply(t *testing.T) {
	withTempHome(t)
	n := 1
	repo := "/repo"
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Body: "please check this", Author: "user",
			Replies: []Reply{{ID: "r_1", Body: "looks fine", Author: "agent"}}},
	}}))
	session, err := loadSession(repo)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, session, nil)
	m.lineIndex = 1

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := mm.(model)
	if m2.mode != modeComment || m2.input != "" || m2.replyingToCommentID != "c_1" || m2.editingCommentID != "" {
		t.Fatalf("expected a blank reply editor targeting c_1, got mode=%v input=%q editing=%q replying=%q",
			m2.mode, m2.input, m2.editingCommentID, m2.replyingToCommentID)
	}

	m2.input = "thanks, will merge"
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := mm.(model)
	if len(m3.session.Comments) != 1 {
		t.Fatalf("expected the reply to attach to the existing thread, not create a new comment — got %d comments", len(m3.session.Comments))
	}
	c := m3.session.Comments[0]
	if c.Body != "please check this" {
		t.Fatalf("expected the original body untouched, got %q", c.Body)
	}
	if len(c.Replies) != 2 || c.Replies[1].Body != "thanks, will merge" || c.Replies[1].Author != "user" {
		t.Fatalf("expected a second reply from user appended, got %+v", c.Replies)
	}
}

// TestAddCommentIgnoresAgentAuthoredComments guards the scoping decision:
// AddComment only offers to edit/reply to the user's own comments — an
// agent-authored one (e.g. via `rv comment reply` creating a top-level
// comment, or any Author != "user") is left alone, and a fresh comment is
// started instead.
func TestAddCommentIgnoresAgentAuthoredComments(t *testing.T) {
	withTempHome(t)
	n := 1
	repo := "/repo"
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Body: "agent note", Author: "agent"},
	}}))
	session, err := loadSession(repo)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, session, nil)
	m.lineIndex = 1

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := mm.(model)
	if m2.editingCommentID != "" || m2.replyingToCommentID != "" || m2.input != "" {
		t.Fatalf("expected an agent-authored comment to be left alone, got editing=%q replying=%q input=%q",
			m2.editingCommentID, m2.replyingToCommentID, m2.input)
	}
}

// TestEditingResolvedCommentUnresolvesIt guards "if I resolve a thread
// before in same line, bring that back" — editing or replying to a
// resolved comment un-resolves it rather than requiring a separate step or
// starting a second, parallel thread on the same line.
func TestEditingResolvedCommentUnresolvesIt(t *testing.T) {
	withTempHome(t)
	n := 1
	repo := "/repo"
	must(t, saveSession(repo, Session{RepoRoot: repo, Comments: []Comment{
		{ID: "c_1", File: "a.go", NewLine: &n, Body: "original", Author: "user", Resolved: true},
	}}))
	session, err := loadSession(repo)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(repo, []FileDiff{fileDiffWithLines("a.go", 3)}, session, nil)
	m.lineIndex = 1

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := mm.(model)
	if m2.editingCommentID != "c_1" {
		t.Fatalf("expected a resolved-but-unreplied comment to still be editable in place, got editing=%q", m2.editingCommentID)
	}

	m2.input = "still relevant"
	mm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := mm.(model)
	if m3.session.Comments[0].Resolved {
		t.Fatalf("expected editing a resolved comment to un-resolve it")
	}
}
