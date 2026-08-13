// Package contract_test holds the six interfaces agent-session promises to other
// programs to the specifications in docs/spec.
//
// These are not ordinary unit tests. A session is written by one agent and read
// by another, often by a different build, so the shapes in docs/spec are the only
// thing both sides can rely on. Prose rots silently; a test does not. Each file
// here carries the frozen v1 baseline as a literal and fails when the shipped
// shape moves away from it — the failure message names the spec to update and
// whether the change needs a version bump.
//
// Adding a field, tool or event type is compatible and still fails these tests
// on purpose: the baseline has to be extended deliberately, in the same commit
// that extends the spec. See docs/spec/README.md for the rule.
package contract_test

import (
	"os/exec"
	"sort"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

// newProject initializes a real project in a temp dir and returns the app.
// Contract tests run against the wiring users get, not a hand-built one.
func newProject(t *testing.T) *bootstrap.App {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	app, err := bootstrap.Init(dir, "claude")
	if err != nil {
		t.Fatalf("init project: %v", err)
	}
	t.Cleanup(func() { app.Store.Close() })
	return app
}

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
}

// diffSets reports what is missing from and extra in got relative to want.
func diffSets(want, got map[string]bool) (missing, extra []string) {
	for k := range want {
		if !got[k] {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if !want[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
