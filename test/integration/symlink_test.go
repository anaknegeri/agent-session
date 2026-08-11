package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t.co")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "init")
}

// TestSymlinkedProjectResolves ensures a project initialized through a symlink
// (e.g. macOS /var -> /private/var) is found when the store is opened via the
// canonical path, and vice versa.
func TestSymlinkedProjectResolves(t *testing.T) {
	real := t.TempDir()
	gitInit(t, real)

	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	ctx := context.Background()

	app, err := bootstrap.Init(link, "claude")
	if err != nil {
		t.Fatalf("init via symlink: %v", err)
	}

	// Opening through the real path must find the same project.
	opened, err := bootstrap.Open(real)
	if err != nil {
		t.Fatalf("open real path: %v", err)
	}
	projectID, err := opened.ResolveProjectID(ctx, real)
	if err != nil {
		t.Fatalf("resolve via real path: %v", err)
	}
	if projectID == "" {
		t.Fatal("expected a project id")
	}

	// And resolving through the symlink path must work too.
	viaLink, err := opened.ResolveProjectID(ctx, link)
	if err != nil {
		t.Fatalf("resolve via symlink path: %v", err)
	}
	if viaLink != projectID {
		t.Fatalf("symlink resolve mismatch: %s != %s", viaLink, projectID)
	}

	_ = app
}
