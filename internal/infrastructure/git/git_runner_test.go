package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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

func TestDiffStatUnbornHeadIsClean(t *testing.T) {
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
	if len(changes) != 0 {
		t.Fatalf("expected no changes on unborn HEAD, got %+v", changes)
	}
	diff, err := r.Diff(ctx, dir)
	if err != nil {
		t.Fatalf("Diff on unborn HEAD should not error: %v", err)
	}
	if diff != "" {
		t.Fatalf("expected empty diff on unborn HEAD, got %q", diff)
	}
}
