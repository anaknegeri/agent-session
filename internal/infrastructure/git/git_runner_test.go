package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t.co")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "init")
	return dir
}

func TestDiffStatIncludesStagedChanges(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	r := NewRunner()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("git", "-C", dir, "add", "a.txt")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}

	changes, err := r.DiffStat(ctx, dir)
	if err != nil {
		t.Fatalf("DiffStat: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "a.txt" {
		t.Fatalf("expected staged change a.txt, got %+v", changes)
	}

	status, err := r.Status(ctx, dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Dirty {
		t.Fatal("expected Dirty=true with a staged change")
	}
	if len(status.Changes) != 1 {
		t.Fatalf("expected 1 change in status, got %+v", status.Changes)
	}

	diff, err := r.Diff(ctx, dir)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff for staged change")
	}
}

// TestDiffStatUnbornHead covers a repository with no commit yet. Neither call may
// error, and an untracked file must be reported: a fresh repo the user has
// already put files in is dirty, and hiding that made auto-record and
// auto-checkpoint treat the very first session as having nothing to save.
// `Diff` still has nothing to show, because there is no HEAD to diff against.
func TestDiffStatUnbornHead(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-q", "-b", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRunner()
	ctx := context.Background()
	changes, err := r.DiffStat(ctx, dir)
	if err != nil {
		t.Fatalf("DiffStat on unborn HEAD should not error: %v", err)
	}
	c, ok := changeFor(changes, "a.txt")
	if !ok {
		t.Fatalf("untracked file not reported on unborn HEAD, got %+v", changes)
	}
	if c.Status != "??" {
		t.Errorf("untracked status = %q, want %q", c.Status, "??")
	}

	diff, err := r.Diff(ctx, dir)
	if err != nil {
		t.Fatalf("Diff on unborn HEAD should not error: %v", err)
	}
	if diff != "" {
		t.Fatalf("expected empty diff on unborn HEAD, got %q", diff)
	}
}

// gitRun runs a git command in dir, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func changeFor(changes []ports.FileChange, path string) (ports.FileChange, bool) {
	for _, c := range changes {
		if c.Path == path {
			return c, true
		}
	}
	return ports.FileChange{}, false
}

// TestDiffStatRenameHasCleanPath covers a rename. `git diff HEAD --name-status`
// prints "R100\told\tnew", so splitting on the first tab left both paths in one
// field and a change whose Path was "old\tnew" — a path that does not exist.
func TestDiffStatRenameHasCleanPath(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	r := NewRunner()

	gitRun(t, dir, "mv", "a.txt", "b.txt")

	changes, err := r.DiffStat(ctx, dir)
	if err != nil {
		t.Fatalf("diff stat: %v", err)
	}
	c, ok := changeFor(changes, "b.txt")
	if !ok {
		t.Fatalf("rename destination b.txt not reported, got %+v", changes)
	}
	if c.Status != "R" {
		t.Errorf("rename status = %q, want %q", c.Status, "R")
	}
	for _, c := range changes {
		if strings.ContainsAny(c.Path, "\t\n") {
			t.Errorf("change path contains whitespace structure: %q", c.Path)
		}
	}
}

// TestDiffStatStatusCodes pins the single-letter contract documented on
// ports.FileChange.Status across staged, unstaged and untracked states.
func TestDiffStatStatusCodes(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	r := NewRunner()

	// commit a file that a later step will delete
	if err := os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "gone.txt")
	gitRun(t, dir, "commit", "-qm", "add gone")

	// staged add
	if err := os.WriteFile(filepath.Join(dir, "added.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "added.txt")
	// unstaged modification
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// staged delete
	gitRun(t, dir, "rm", "-q", "gone.txt")
	// untracked
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := r.DiffStat(ctx, dir)
	if err != nil {
		t.Fatalf("diff stat: %v", err)
	}

	want := map[string]string{
		"added.txt":     "A",
		"a.txt":         "M",
		"gone.txt":      "D",
		"untracked.txt": "??",
	}
	for path, status := range want {
		c, ok := changeFor(changes, path)
		if !ok {
			t.Errorf("%s not reported, got %+v", path, changes)
			continue
		}
		if c.Status != status {
			t.Errorf("%s status = %q, want %q", path, c.Status, status)
		}
	}
}

// TestStatusOnUnbornHead verifies a repository with no commit yet reports its
// branch and no commit, rather than failing.
func TestStatusOnUnbornHead(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	ctx := context.Background()
	r := NewRunner()

	st, err := r.Status(ctx, dir)
	if err != nil {
		t.Fatalf("status on unborn head: %v", err)
	}
	if st.Branch != "main" {
		t.Errorf("branch = %q, want main", st.Branch)
	}
	if st.Commit != "" {
		t.Errorf("commit = %q, want empty on unborn HEAD", st.Commit)
	}
}

// TestStatusDetachedHead verifies a detached HEAD does not leak the "(detached)"
// placeholder as a branch name.
func TestStatusDetachedHead(t *testing.T) {
	dir := gitRepo(t)
	gitRun(t, dir, "checkout", "-q", "--detach", "HEAD")
	ctx := context.Background()
	r := NewRunner()

	st, err := r.Status(ctx, dir)
	if err != nil {
		t.Fatalf("status detached: %v", err)
	}
	if st.Branch == "(detached)" {
		t.Errorf("branch leaked the porcelain placeholder: %q", st.Branch)
	}
	if st.Commit == "" {
		t.Error("commit should still resolve on a detached HEAD")
	}
}
