package services_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
			app := fx.openApp(t)
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
			app := fx.openApp(t)
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
			app := fx.openApp(t)
			if err := app.Event.Append(ctx, fx.sessionID, fmt.Sprintf("agent-%d", w), "file.changed", ""); err != nil {
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

// TestRecoveryAfterInterruptedWrite simulates a process killed mid-transaction:
// the rolled-back work must be absent and the store must stay usable.
func TestRecoveryAfterInterruptedWrite(t *testing.T) {
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
	if err := next.Event.Append(ctx, fx.sessionID, "claude", "test.passed", ""); err != nil {
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
