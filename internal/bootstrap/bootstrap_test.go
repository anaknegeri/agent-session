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

// TestFindRootStopsAtRepositoryBoundary covers a real hazard of walking up until
// the filesystem root: a developer who once ran `agent-session init` in $HOME has
// a .agent/ directory above every one of their repositories. Every uninitialized
// repo underneath it then resolved to that unrelated project, so a session would
// silently be recorded against the wrong one.
func TestFindRootStopsAtRepositoryBoundary(t *testing.T) {
	outer := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outer, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(outer, "unrelated-repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "pkg", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveRoot(nested)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	want := resolvePath(t, repo)
	if got != want {
		t.Errorf("root = %q, want the repository root %q — the search escaped the repo and found an unrelated .agent/", got, want)
	}
}

// TestFindRootUsesEnvOverride covers the explicit escape hatch the review asks
// for: user-scope MCP registration means the server can start in a directory that
// has nothing to do with the project being worked on.
func TestFindRootUsesEnvOverride(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()

	t.Setenv("AGENT_SESSION_PROJECT", project)
	got, err := ResolveRoot(elsewhere)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if got != resolvePath(t, project) {
		t.Errorf("root = %q, want the override %q", got, resolvePath(t, project))
	}
}

// TestFindRootIgnoresUnusableOverride verifies a stale or misspelled override does
// not silently hijack discovery.
func TestFindRootIgnoresUnusableOverride(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AGENT_SESSION_PROJECT", filepath.Join(project, "does-not-exist"))
	got, err := ResolveRoot(project)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if got != resolvePath(t, project) {
		t.Errorf("root = %q, want discovery to fall back to %q", got, resolvePath(t, project))
	}
}

func resolvePath(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
