package bootstrap

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr runs fn and returns everything written to stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()
	w.Close()
	data, _ := io.ReadAll(r)
	os.Stderr = old
	return string(data)
}

func TestInitRemindsGitInitWhenNotGitRepo(t *testing.T) {
	dir := t.TempDir() // no git

	stderr := captureStderr(t, func() {
		app, err := Init(dir, "cli")
		if err != nil {
			t.Fatalf("init in non-git dir should succeed (local mode): %v", err)
		}
		defer app.Store.Close()
	})

	if !strings.Contains(stderr, "git init") {
		t.Fatalf("expected git init reminder, stderr: %q", stderr)
	}
	if !strings.Contains(stderr, "local-only") {
		t.Fatalf("expected local-only note, stderr: %q", stderr)
	}
}

func TestInitNoReminderWhenGitRepo(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.email", "t@t.co")
	git(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "init")

	stderr := captureStderr(t, func() {
		app, err := Init(dir, "cli")
		if err != nil {
			t.Fatalf("init: %v", err)
		}
		defer app.Store.Close()
	})

	if strings.Contains(stderr, "git init") {
		t.Fatalf("expected no reminder for git repo, stderr: %q", stderr)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
