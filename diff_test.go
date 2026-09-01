package main

import (
	"reflect"
	"strings"
	"testing"
)

type fakeGitRunner struct {
	calls  [][]string
	output string
	err    error
}

func (f *fakeGitRunner) Run(args ...string) (string, error) {
	f.calls = append(f.calls, args)
	return f.output, f.err
}

func TestGitVCSDiffUsesHEAD(t *testing.T) {
	runner := &fakeGitRunner{output: "diff --git a/x b/x\n"}
	vcs := gitVCS{runner: runner}

	out, err := vcs.Diff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != runner.output {
		t.Fatalf("got %q, want %q", out, runner.output)
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != "diff" || runner.calls[0][1] != "HEAD" {
		t.Fatalf("unexpected git invocation: %v", runner.calls)
	}
}

func TestGitVCSDiffUsesCustomSpec(t *testing.T) {
	runner := &fakeGitRunner{output: "diff --git a/x b/x\n"}
	vcs := gitVCS{runner: runner, spec: []string{"main..feature"}}

	if _, err := vcs.Diff(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != "diff" || runner.calls[0][1] != "main..feature" {
		t.Fatalf("unexpected git invocation: %v", runner.calls)
	}
}

func TestGitVCSDiffSpecStagedPassedThrough(t *testing.T) {
	runner := &fakeGitRunner{output: ""}
	vcs := gitVCS{runner: runner, spec: []string{"--staged"}}

	if _, err := vcs.Diff(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.calls[0][1] != "--staged" {
		t.Fatalf("unexpected git invocation: %v", runner.calls)
	}
}

func TestGitVCSRepoRoot(t *testing.T) {
	runner := &fakeGitRunner{output: "/repo/root\n"}
	vcs := gitVCS{runner: runner}

	root, err := vcs.RepoRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != "/repo/root" {
		t.Fatalf("got %q, want %q (should trim trailing newline)", root, "/repo/root")
	}
}

// Built with explicit " " context-line prefixes rather than a raw string
// literal, since editors/formatters happily strip the trailing space off an
// otherwise-blank line and silently turn it into an invalid diff.
var modifiedFileDiff = strings.Join([]string{
	"diff --git a/foo.go b/foo.go",
	"index 1111111..2222222 100644",
	"--- a/foo.go",
	"+++ b/foo.go",
	"@@ -1,4 +1,5 @@",
	" package main",
	" ",
	"-func old() {}",
	"+func newFunc() {}",
	"+func another() {}",
	" ",
	"",
}, "\n")

func TestParseDiffModifiedFile(t *testing.T) {
	files, err := ParseDiff(modifiedFileDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Path != "foo.go" || f.Status != FileModified {
		t.Fatalf("unexpected file: %+v", f)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(f.Hunks))
	}

	lines := f.Hunks[0].Lines
	want := []struct {
		kind    LineKind
		content string
		oldLine *int
		newLine *int
	}{
		{LineContext, "package main", intp(1), intp(1)},
		{LineContext, "", intp(2), intp(2)},
		{LineRemoved, "func old() {}", intp(3), nil},
		{LineAdded, "func newFunc() {}", nil, intp(3)},
		{LineAdded, "func another() {}", nil, intp(4)},
		{LineContext, "", intp(4), intp(5)},
	}
	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %+v", len(want), len(lines), lines)
	}
	for i, w := range want {
		l := lines[i]
		if l.Kind != w.kind || l.Content != w.content {
			t.Fatalf("line %d: got kind=%v content=%q, want kind=%v content=%q", i, l.Kind, l.Content, w.kind, w.content)
		}
		if !reflect.DeepEqual(l.OldLine, w.oldLine) {
			t.Fatalf("line %d: OldLine got %v want %v", i, deref(l.OldLine), deref(w.oldLine))
		}
		if !reflect.DeepEqual(l.NewLine, w.newLine) {
			t.Fatalf("line %d: NewLine got %v want %v", i, deref(l.NewLine), deref(w.newLine))
		}
	}
}

const newFileDiff = `diff --git a/bar.go b/bar.go
new file mode 100644
index 0000000..1111111
--- /dev/null
+++ b/bar.go
@@ -0,0 +1,2 @@
+package main
+
`

func TestParseDiffNewFile(t *testing.T) {
	files, err := ParseDiff(newFileDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "bar.go" || files[0].Status != FileAdded {
		t.Fatalf("unexpected files: %+v", files)
	}
}

const deletedFileDiff = `diff --git a/baz.go b/baz.go
deleted file mode 100644
index 1111111..0000000
--- a/baz.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package main
-
`

func TestParseDiffDeletedFile(t *testing.T) {
	files, err := ParseDiff(deletedFileDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "baz.go" || files[0].Status != FileDeleted {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func intp(n int) *int { return &n }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
