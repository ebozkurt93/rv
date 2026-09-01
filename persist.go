package main

import (
	"encoding/json"
	"os"
	"syscall"
	"time"
)

// loadSession reads the session for repoRoot, returning a fresh empty
// session (not an error) if none exists yet.
func loadSession(repoRoot string) (Session, error) {
	path := sessionPath(repoRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Session{RepoRoot: repoRoot, CreatedAt: time.Now()}, nil
	}
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, err
	}
	return s, nil
}

// sessionModTime returns the on-disk session's last-modified time, or the
// zero time (with no error) if it doesn't exist yet — so a caller can always
// compare with ".After()" without special-casing "no session written yet".
// Used by the TUI to poll for changes made by another process (e.g. an
// agent running `rv comment reply`) while it's sitting open.
func sessionModTime(repoRoot string) (time.Time, error) {
	info, err := os.Stat(sessionPath(repoRoot))
	if os.IsNotExist(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// saveSession writes s for repoRoot, atomically: write to a temp file in the
// same directory, then rename into place, so a concurrent reader never
// observes a half-written file.
func saveSession(repoRoot string, s Session) error {
	dir := sessionDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := sessionPath(repoRoot)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// withSession runs fn against the current session for repoRoot under an
// exclusive flock, persisting whatever fn returns. This is the transaction
// boundary for every read-modify-write (add/resolve/delete a comment) so the
// TUI and a concurrently-running `rv comment ...` invocation can't clobber
// each other's changes.
func withSession(repoRoot string, fn func(Session) (Session, error)) error {
	dir := sessionDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	lock, err := os.OpenFile(sessionLockPath(repoRoot), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	s, err := loadSession(repoRoot)
	if err != nil {
		return err
	}
	next, err := fn(s)
	if err != nil {
		return err
	}
	return saveSession(repoRoot, next)
}
