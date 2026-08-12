package services_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

// diskFixture creates a real git repo and initializes agent-session on disk,
// so multiple app instances can open the same SQLite DB (process-like sharing).
type diskFixture struct {
	dir       string
	sessionID string
	projectID string
}

func newDiskFixture(t *testing.T) *diskFixture {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "config", "user.email", "t@t.co")
	gitCmd(t, dir, "config", "user.name", "t")
	writeFile(t, dir, "server.go", "package oauth\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-qm", "init")

	app, err := bootstrap.Init(dir, "claude")
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	projectID, err := app.ResolveProjectID(context.Background(), dir)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.GetActive(context.Background(), projectID)
	if err != nil {
		t.Fatalf("get active session: %v", err)
	}
	app.Store.Close()

	return &diskFixture{dir: dir, sessionID: session.ID, projectID: projectID}
}

// openApp opens a fresh app instance on the shared on-disk store, simulating
// another agent/CLI process.
func (f *diskFixture) openApp(t *testing.T) *bootstrap.App {
	t.Helper()
	app, err := bootstrap.Open(f.dir)
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	t.Cleanup(func() { app.Store.Close() })
	return app
}

// TestConcurrentEventsAppend spawns N goroutines (each its own app instance on
// the same on-disk DB) appending events concurrently. SQLite WAL + busy
// timeout must handle the contention.
func TestConcurrentEventsAppend(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	const writers = 8
	const perWriter = 20
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app2 := fx.openApp(t)
			for i := 0; i < perWriter; i++ {
				if err := app2.Event.Append(ctx, fx.sessionID, fmt.Sprintf("agent-%d", w), "file.changed", ""); err != nil {
					errs <- fmt.Errorf("writer %d append %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	app2 := fx.openApp(t)
	events, err := app2.Event.List(ctx, fx.sessionID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// count only events appended by the writers (agent-N prefix), ignoring the
	// bootstrap/sync events (session.started, auto-recorded file.changed)
	writerEvents := 0
	for _, e := range events {
		if len(e.Agent) >= 6 && e.Agent[:6] == "agent-" {
			writerEvents++
		}
	}
	if writerEvents != writers*perWriter {
		t.Fatalf("expected %d writer events, got %d", writers*perWriter, writerEvents)
	}
}

// TestConcurrentCheckpoints spawns writers creating checkpoints concurrently.
func TestConcurrentCheckpoints(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	const writers = 6
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app2 := fx.openApp(t)
			for i := 0; i < 5; i++ {
				if _, err := app2.Checkpoint.Create(ctx, fx.sessionID, fmt.Sprintf("w%d-%d", w, i), "", "agent"); err != nil {
					errs <- fmt.Errorf("writer %d checkpoint %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	app2 := fx.openApp(t)
	cps, err := app2.Checkpoint.ListBySession(ctx, fx.sessionID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// writer checkpoints use the w%d-%d label; ignore bootstrap auto-checkpoints
	writerCPs := 0
	for _, c := range cps {
		if len(c.Label) >= 2 && c.Label[0] == 'w' && c.Label[1] >= '0' && c.Label[1] <= '9' {
			writerCPs++
		}
	}
	if writerCPs != writers*5 {
		t.Fatalf("expected %d writer checkpoints, got %d", writers*5, writerCPs)
	}
}

// TestConcurrentResumeRaces verifies concurrent resumes keep exactly one open
// agent session and the session stays active.
func TestConcurrentResumeRaces(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	const writers = 5
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app2 := fx.openApp(t)
			if _, err := app2.Session.Resume(ctx, fx.projectID, fmt.Sprintf("agent-%d", w)); err != nil {
				errs <- fmt.Errorf("resume writer %d: %w", w, err)
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	app2 := fx.openApp(t)
	rows, err := app2.Store.AgentSessions().ListOpen(ctx, fx.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 open agent session after concurrent resumes, got %d", len(rows))
	}

	sess, err := app2.Session.Get(ctx, fx.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != "active" {
		t.Fatalf("expected active session, got %q", sess.Status)
	}
}

// TestMigrationWhileOpen verifies a second process opening the same DB (which
// runs the idempotent migration) while writes happen does not fail.
func TestMigrationWhileOpen(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	app2 := fx.openApp(t) // runs migration on open
	if err := app2.Event.Append(ctx, fx.sessionID, "claude", "file.changed", ""); err != nil {
		t.Fatalf("append via second app: %v", err)
	}
	app1 := fx.openApp(t)
	if err := app1.Event.Append(ctx, fx.sessionID, "claude", "file.changed", ""); err != nil {
		t.Fatalf("append via first app: %v", err)
	}
}
