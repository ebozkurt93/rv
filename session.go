package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Session is the persisted review state for one repo checkout: comments plus
// enough metadata to know where they belong. The diff itself is never
// persisted here — it's recomputed from git on every read, so a session
// never goes stale relative to the working tree.
type Session struct {
	RepoRoot  string    `json:"repo_root"`
	CreatedAt time.Time `json:"created_at"`
	Comments  []Comment `json:"comments"`
}

// Comment is a single review note anchored to a line in a diff. OldLine
// and/or NewLine are set depending on which side(s) of the diff the
// commented-on line belongs to (see Line in diff.go) — a comment on a
// removed line has only OldLine, on an added line only NewLine, and on a
// context line both.
type Comment struct {
	ID        string    `json:"id"`
	File      string    `json:"file"`
	OldLine   *int      `json:"old_line,omitempty"`
	NewLine   *int      `json:"new_line,omitempty"`
	Body      string    `json:"body"`
	Author    string    `json:"author"` // "user" (left in the TUI) or "agent" (left via `rv comment reply`)
	Resolved  bool      `json:"resolved"`
	CreatedAt time.Time `json:"created_at"`
	Replies   []Reply   `json:"replies,omitempty"`
}

// Reply is a threaded response to a Comment — how an agent talks back
// ("fixed, see foo.go:12" or similar) without the reply itself needing its
// own line anchor.
type Reply struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// cacheDir returns ~/.cache/rv, matching tmux-mover's ~/.cache/<name>
// convention (a plain $HOME join rather than os.UserCacheDir, so behavior is
// predictable and inspectable rather than platform-dependent).
func cacheDir() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "rv")
}

// sessionDir returns the per-repo directory holding a repo's session.json
// and its lock file, keyed by a short hash of the repo's absolute root path
// so distinct checkouts/worktrees of the same repo never collide.
func sessionDir(repoRoot string) string {
	sum := sha256.Sum256([]byte(repoRoot))
	return filepath.Join(cacheDir(), hex.EncodeToString(sum[:])[:16])
}

func sessionPath(repoRoot string) string {
	return filepath.Join(sessionDir(repoRoot), "session.json")
}

func sessionLockPath(repoRoot string) string {
	return filepath.Join(sessionDir(repoRoot), "session.lock")
}

// newCommentID returns a short random, collision-safe-enough id for a
// comment created within a single session.
func newCommentID() string {
	return "c_" + randomHex()
}

// newReplyID is newCommentID's counterpart for Reply.ID.
func newReplyID() string {
	return "r_" + randomHex()
}

func randomHex() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
