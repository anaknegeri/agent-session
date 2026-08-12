package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BenchmarkStatus measures one workspace status against a real repository.
// Status is on the hot path: every context.get calls it, so the subprocess count
// is what this benchmark exists to keep honest. Collecting branch, HEAD and the
// change set from a single `git status --porcelain=v2 --branch` took it from six
// git processes to three (two when HEAD is unborn), roughly halving the latency.
func BenchmarkStatus(b *testing.B) {
	dir := b.TempDir()
	sh := func(args ...string) {
		b.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	sh("init", "-q", "-b", "main")
	sh("config", "user.email", "t@t.co")
	sh("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	sh("add", ".")
	sh("commit", "-qm", "init")

	// a modified tracked file and an untracked one, so the parser has work to do
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("y\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "u.txt"), []byte("z\n"), 0o644); err != nil {
		b.Fatal(err)
	}

	r := NewRunner()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Status(ctx, dir); err != nil {
			b.Fatal(err)
		}
	}
}
