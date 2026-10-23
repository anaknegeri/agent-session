package integration

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

// TestCrossProcessConcurrentEventAppend is the one thing every other
// concurrency test in this repo cannot show: those are goroutines sharing one
// process, which share a connection pool and a Go runtime scheduler that a
// real second agent process does not. This launches N real OS processes,
// each the actual agent-session CLI, writing to the same on-disk project at
// once, and checks that every write landed exactly once with none missing or
// duplicated.
func TestCrossProcessConcurrentEventAppend(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir, "--no-agents")
	bin := filepath.Join(binDir(t), "agent-session")

	const writers = 6
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			cmd := exec.Command(bin, "event", "add", "file.changed",
				"--agent", fmt.Sprintf("writer-%d", w),
				"--payload", fmt.Sprintf(`{"writer":%d}`, w))
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				errs <- fmt.Errorf("writer %d: %v: %s", w, err, out)
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}

	app, err := bootstrap.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer app.Store.Close()
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, dir)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.GetActive(ctx, projectID)
	if err != nil {
		t.Fatalf("get active session: %v", err)
	}
	events, err := app.Event.List(ctx, session.ID, 1000)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	// Each writer's event is attributable by its own agent name (writer-N), so
	// missing and duplicated rows are both visible as a count other than 1 —
	// ignoring the bootstrap-recorded session.started/file.changed events,
	// which carry the "cli" agent from `init`.
	seen := make(map[int]int, writers)
	for _, e := range events {
		var idx int
		if n, _ := fmt.Sscanf(e.Agent, "writer-%d", &idx); n == 1 {
			seen[idx]++
		}
	}
	for w := 0; w < writers; w++ {
		if seen[w] != 1 {
			t.Errorf("writer %d: expected exactly 1 event, got %d", w, seen[w])
		}
	}
	if len(seen) != writers {
		t.Errorf("expected events attributable to exactly %d writers, saw %d", writers, len(seen))
	}
}
