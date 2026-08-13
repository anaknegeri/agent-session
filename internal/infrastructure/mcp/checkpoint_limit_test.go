package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// TestSmartCheckpointLimitUnderConcurrency drives the auto-checkpoint rate
// limiter from several goroutines at once, which is how it is actually reached:
// streamable-HTTP serves tool calls concurrently and every write tool funnels
// into maybeCheckpoint.
//
// lastCheckpoint used to be read and written with no lock, so callers could all
// see the same stale timestamp and each checkpoint anyway — a window that held
// only for a single-threaded client. MUST be run with -race: that is what proves
// the accesses are synchronised, since SQLite's single writer hides the extra
// checkpoints most of the time. The count below holds the limit itself.
func TestSmartCheckpointLimitUnderConcurrency(t *testing.T) {
	dir := t.TempDir()
	gitRepoForTest(t, dir)
	app, err := bootstrap.Init(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !app.Cfg.Session.AutoCheckpoint || !app.Cfg.Session.SmartCheckpoint {
		t.Fatalf("smart auto-checkpoint is off in this project, nothing to rate-limit: %+v", app.Cfg.Session)
	}

	s := New(dir, logger.New("error"))
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := app.Session.GetActive(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := app.Checkpoint.ListBySession(ctx, session.ID, 1000)
	if err != nil {
		t.Fatal(err)
	}

	const callers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if err := s.maybeCheckpoint(ctx, session.ID, fmt.Sprintf("concurrent call %d", i)); err != nil {
				t.Errorf("maybeCheckpoint %d: %v", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	after, err := app.Checkpoint.ListBySession(ctx, session.ID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if created := len(after) - len(before); created > 1 {
		t.Errorf("%d concurrent callers created %d checkpoints; the %s window admits 1", callers, created, smartCheckpointWindow)
	}
}

// gitRepoForTest gives the project a repository, since the checkpoint snapshot
// reads git for the workspace section.
func gitRepoForTest(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@t.co"},
		{"config", "user.name", "t"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
