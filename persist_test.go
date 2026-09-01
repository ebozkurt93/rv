package main

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	prev := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", prev) })
}

func TestSessionDirIsStablePerRepoRoot(t *testing.T) {
	a := sessionDir("/repo/one")
	b := sessionDir("/repo/one")
	c := sessionDir("/repo/two")
	if a != b {
		t.Fatalf("expected same repo root to hash to the same dir, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different repo roots to hash to different dirs, both got %q", a)
	}
}

func TestLoadSessionMissingReturnsEmpty(t *testing.T) {
	withTempHome(t)

	s, err := loadSession("/repo/one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.RepoRoot != "/repo/one" || len(s.Comments) != 0 {
		t.Fatalf("unexpected empty session: %+v", s)
	}
}

func TestSaveAndLoadSessionRoundTrip(t *testing.T) {
	withTempHome(t)

	n := 1
	want := Session{
		RepoRoot: "/repo/one",
		Comments: []Comment{
			{ID: "c_1", File: "foo.go", NewLine: &n, Body: "looks off", Author: "user"},
		},
	}
	if err := saveSession(want.RepoRoot, want); err != nil {
		t.Fatalf("saveSession error: %v", err)
	}

	got, err := loadSession(want.RepoRoot)
	if err != nil {
		t.Fatalf("loadSession error: %v", err)
	}
	if len(got.Comments) != 1 || got.Comments[0].ID != "c_1" || *got.Comments[0].NewLine != 1 {
		t.Fatalf("unexpected roundtrip result: %+v", got)
	}
}

func TestSaveSessionIsAtomic(t *testing.T) {
	withTempHome(t)

	if err := saveSession("/repo/one", Session{RepoRoot: "/repo/one"}); err != nil {
		t.Fatalf("saveSession error: %v", err)
	}
	tmp := sessionPath("/repo/one") + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected the .tmp file to be renamed away, stat error: %v", err)
	}
}

func TestWithSessionPersistsMutation(t *testing.T) {
	withTempHome(t)

	err := withSession("/repo/one", func(s Session) (Session, error) {
		s.Comments = append(s.Comments, Comment{ID: "c_1", File: "foo.go"})
		return s, nil
	})
	if err != nil {
		t.Fatalf("withSession error: %v", err)
	}

	s, err := loadSession("/repo/one")
	if err != nil {
		t.Fatalf("loadSession error: %v", err)
	}
	if len(s.Comments) != 1 || s.Comments[0].ID != "c_1" {
		t.Fatalf("unexpected session after withSession: %+v", s)
	}
}

// TestWithSessionSerializesConcurrentWriters exercises the flock: many
// goroutines each append one comment under withSession, and none of their
// updates should be lost to a lost-update race.
func TestWithSessionSerializesConcurrentWriters(t *testing.T) {
	withTempHome(t)

	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := withSession("/repo/one", func(s Session) (Session, error) {
				s.Comments = append(s.Comments, Comment{ID: fmt.Sprintf("c_%d", i)})
				return s, nil
			})
			if err != nil {
				t.Errorf("withSession error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	s, err := loadSession("/repo/one")
	if err != nil {
		t.Fatalf("loadSession error: %v", err)
	}
	if len(s.Comments) != n {
		t.Fatalf("expected %d comments, got %d (lost update under concurrent writers)", n, len(s.Comments))
	}
}
