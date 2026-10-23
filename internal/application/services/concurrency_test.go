package services_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/infrastructure/database"
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
// another agent/CLI process. Fails the test immediately, so call this only
// from the test's own goroutine: t.Fatalf's runtime.Goexit unwinds just the
// calling goroutine, and from inside a worker goroutine that means the error
// vanishes silently while the test reports a false pass. Worker goroutines
// must use openAppOrErr and report through their own errs channel instead.
func (f *diskFixture) openApp(t *testing.T) *bootstrap.App {
	t.Helper()
	app, err := f.openAppOrErr(t)
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	return app
}

// openAppOrErr is the goroutine-safe variant of openApp: it returns the error
// instead of failing the test. t.Helper and t.Cleanup are safe to call from any
// goroutine while the test is still running, unlike t.Fatalf/t.FailNow.
func (f *diskFixture) openAppOrErr(t *testing.T) (*bootstrap.App, error) {
	t.Helper()
	app, err := bootstrap.Open(f.dir)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { app.Store.Close() })
	return app, nil
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
			app2, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("writer %d open: %w", w, err)
				return
			}
			for i := 0; i < perWriter; i++ {
				if err := app2.Artifact.AppendEvent(ctx, fx.sessionID, fmt.Sprintf("agent-%d", w), "file.changed", ""); err != nil {
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
			app2, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("writer %d open: %w", w, err)
				return
			}
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
			app2, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("resume writer %d open: %w", w, err)
				return
			}
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

// TestMigrationWhileOpen is deleted: it was fully sequential (open, append,
// open, append — no goroutines, no write in flight), so despite its name it
// never exercised migration racing a concurrent write; it would not have
// failed if the BEGIN IMMEDIATE guarding migration were removed. The real
// race — several processes opening and migrating a never-before-migrated (or
// newly-pending) database at once — is already covered by
// TestMigrateConcurrentFresh and TestMigrateConcurrentPendingUpgrade in
// migration_test.go, which run that exact guard under real contention.

// TestConcurrentKnowledgeWrites covers memory promotion and puts happening from
// several processes at once. Knowledge writes touch two tables — knowledge and
// the FTS index — so a torn write would leave the index disagreeing with the rows.
func TestConcurrentKnowledgeWrites(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	const writers = 6
	const perWriter = 10
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("writer %d open: %w", w, err)
				return
			}
			for i := 0; i < perWriter; i++ {
				content := fmt.Sprintf("writer %d fact %d about rotating refresh tokens", w, i)
				if _, err := app.Memory.Put(ctx, fx.sessionID, entities.KnowledgeKindArchitecture, content, fmt.Sprintf("agent-%d", w)); err != nil {
					errs <- fmt.Errorf("writer %d put %d: %w", w, i, err)
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

	app := fx.openApp(t)
	hits, err := app.Memory.Search(ctx, "rotating refresh tokens", 1000)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != writers*perWriter {
		t.Errorf("FTS index holds %d rows, want %d — the index and the table disagree",
			len(hits), writers*perWriter)
	}
}

// TestConcurrentStartRaces covers several processes calling session.start at once.
// Start completes any active session before creating a new one, so racing callers
// must still leave exactly one active session for the project.
func TestConcurrentStartRaces(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	const writers = 5
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("start %d open: %w", w, err)
				return
			}
			if _, err := app.Session.Start(ctx, fx.projectID, fmt.Sprintf("session %d", w), fmt.Sprintf("agent-%d", w)); err != nil {
				errs <- fmt.Errorf("start %d: %w", w, err)
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	app := fx.openApp(t)
	active, err := app.Store.Sessions().ListByProject(ctx, fx.projectID, 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	activeCount := 0
	for _, s := range active {
		if s.Status == entities.SessionStatusActive {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active session after concurrent starts, got %d", activeCount)
	}
}

// TestLockContentionUnderLongWrite holds a write transaction open while other
// processes try to write, verifying busy_timeout absorbs the contention instead of
// surfacing SQLITE_BUSY to callers.
func TestLockContentionUnderLongWrite(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	conn := rawConn(t, fx.dir)
	defer conn.Close()

	// take the write lock and hold it briefly, shorter than busy_timeout
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		conn.ExecContext(ctx, "COMMIT")
		close(released)
	}()

	const writers = 4
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("writer %d open: %w", w, err)
				return
			}
			if err := app.Artifact.AppendEvent(ctx, fx.sessionID, fmt.Sprintf("agent-%d", w), "file.changed", ""); err != nil {
				errs <- fmt.Errorf("writer %d blocked out: %w", w, err)
			}
		}(w)
	}
	wg.Wait()
	<-released
	close(errs)
	for e := range errs {
		t.Errorf("busy_timeout did not absorb lock contention: %v", e)
	}
}

// TestStoreUsableAfterSiblingConnectionAborts simulates a process killed
// mid-transaction: a second, independent connection begins a write, inserts a
// row, and issues an explicit ROLLBACK — SQLite's own rollback guarantees that
// row is gone regardless of agent-session's code. What this test actually
// verifies is agent-session's own property: the store's pooled connections are
// not left wedged by a sibling connection's aborted write. A fresh app
// instance must see none of the abandoned row (no torn read through the shared
// WAL) and must still be able to write afterward.
func TestStoreUsableAfterSiblingConnectionAborts(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	app := fx.openApp(t)
	before, err := app.Event.List(ctx, fx.sessionID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	conn := rawConn(t, fx.dir)

	// write inside a transaction, then abandon it the way a killed process would
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO session_events (id, session_id, agent, type, created_at) VALUES ('evt_torn', ?, 'ghost', 'file.changed', '2026-01-01T00:00:00Z')`,
		fx.sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	// a fresh process must see none of the abandoned work and must still write
	next := fx.openApp(t)
	after, err := next.Event.List(ctx, fx.sessionID, 1000)
	if err != nil {
		t.Fatalf("store unusable after an interrupted write: %v", err)
	}
	for _, e := range after {
		if e.ID == "evt_torn" {
			t.Error("an abandoned transaction left its row behind")
		}
	}
	if len(after) != len(before) {
		t.Errorf("event count changed across a rolled-back write: %d -> %d", len(before), len(after))
	}
	if err := next.Artifact.AppendEvent(ctx, fx.sessionID, "claude", "test.passed", ""); err != nil {
		t.Errorf("store not writable after recovery: %v", err)
	}
}

// rawConn opens a second connection straight to the project's SQLite file, so a
// test can hold a transaction the way an independent process would. Going through
// ports.Store would mean widening that interface for test convenience alone.
func rawConn(t *testing.T, dir string) *sql.Conn {
	t.Helper()
	db, err := database.Open(filepath.Join(dir, ".agent", "session.db"))
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("raw conn: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return conn
}

// TestConcurrentHandoffRaces spawns goroutines calling handoff to different
// target agents on the same session concurrently. Handoff sets the session's
// owning agent unconditionally (not based on its previous value), so
// busy_timeout serializing the writes must leave exactly one target as the
// final owner, and every successful call must leave behind its own handoff
// checkpoint — a lost write here would show up as a missing checkpoint.
func TestConcurrentHandoffRaces(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	targets := []string{"claude", "codex", "opencode", "pi", "omp"}
	var wg sync.WaitGroup
	errs := make(chan error, len(targets))

	for _, to := range targets {
		wg.Add(1)
		go func(to string) {
			defer wg.Done()
			app, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("handoff to %s open: %w", to, err)
				return
			}
			if _, err := app.Handoff.Handoff(ctx, fx.sessionID, to); err != nil {
				errs <- fmt.Errorf("handoff to %s: %w", to, err)
			}
		}(to)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	app := fx.openApp(t)
	sess, err := app.Session.Get(ctx, fx.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	owner := ""
	for _, to := range targets {
		if sess.LastAgent == to {
			owner = to
			break
		}
	}
	if owner == "" {
		t.Fatalf("session.LastAgent = %q, want one of %v", sess.LastAgent, targets)
	}

	cps, err := app.Checkpoint.ListBySession(ctx, fx.sessionID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	handoffCPs := 0
	for _, c := range cps {
		if c.Kind == entities.CheckpointKindHandoff {
			handoffCPs++
		}
	}
	if handoffCPs != len(targets) {
		t.Errorf("expected %d handoff checkpoints (one per successful handoff), got %d", len(targets), handoffCPs)
	}
}

// TestConcurrentArtifactOffload spawns writers each appending an oversized
// (>8KiB) payload concurrently. AppendEvent commits the artifact and the event
// referencing it in one transaction, so under contention every artifact row
// this test creates must have exactly one event pointing back at it: never
// zero (an artifact a half-applied write left with nothing pointing at it)
// and never more than one (two events sharing, and one of them losing, the
// same offloaded payload).
func TestConcurrentArtifactOffload(t *testing.T) {
	fx := newDiskFixture(t)
	ctx := context.Background()

	const writers = 6
	padding := strings.Repeat("x", services.LargePayloadThreshold)
	payloadFor := func(w int) string {
		return fmt.Sprintf(`{"writer":%d,"padding":%q}`, w, padding)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			app, err := fx.openAppOrErr(t)
			if err != nil {
				errs <- fmt.Errorf("writer %d open: %w", w, err)
				return
			}
			if err := app.Artifact.AppendEvent(ctx, fx.sessionID, fmt.Sprintf("agent-%d", w), "file.changed", payloadFor(w)); err != nil {
				errs <- fmt.Errorf("writer %d append: %w", w, err)
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	app := fx.openApp(t)
	events, err := app.Event.List(ctx, fx.sessionID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// map writer index -> artifact_id, decoded from the reference AppendEvent
	// leaves in the event's payload once it offloads.
	refs := make(map[int]string, writers)
	for _, e := range events {
		if !(len(e.Agent) >= 6 && e.Agent[:6] == "agent-") || e.Type != "file.changed" {
			continue
		}
		var w int
		if n, _ := fmt.Sscanf(e.Agent, "agent-%d", &w); n != 1 {
			continue
		}
		var ref struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal([]byte(e.Payload), &ref); err != nil || ref.ArtifactID == "" {
			t.Errorf("writer %d: event payload is not an artifact reference: %q", w, e.Payload)
			continue
		}
		if prior, ok := refs[w]; ok {
			t.Errorf("writer %d: two events reference artifacts (%s and %s)", w, prior, ref.ArtifactID)
			continue
		}
		refs[w] = ref.ArtifactID
	}
	if len(refs) != writers {
		t.Fatalf("expected %d writers with an offloaded artifact reference, got %d", writers, len(refs))
	}

	// every referenced artifact must exist, belong to this session, and hold
	// exactly the payload its writer sent — proving no cross-writer overwrite.
	seenArtifacts := make(map[string]bool, writers)
	for w, artifactID := range refs {
		if seenArtifacts[artifactID] {
			t.Errorf("artifact %s referenced by more than one writer", artifactID)
		}
		seenArtifacts[artifactID] = true
		art, err := app.Artifact.Get(ctx, fx.sessionID, artifactID)
		if err != nil {
			t.Errorf("writer %d: artifact %s not found: %v", w, artifactID, err)
			continue
		}
		if art.Content != payloadFor(w) {
			t.Errorf("writer %d: artifact content does not match what it sent", w)
		}
	}

	// no artifact this test created is orphaned: the session must hold exactly
	// as many artifact rows as there are references, not more.
	conn := rawConn(t, fx.dir)
	defer conn.Close()
	var artifactCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE session_id = ?`, fx.sessionID).Scan(&artifactCount); err != nil {
		t.Fatal(err)
	}
	if artifactCount != len(refs) {
		t.Errorf("session has %d artifact rows, want exactly %d (one per referenced writer, no orphans)", artifactCount, len(refs))
	}
}
