package database

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TestMigrateFresh applies migrations to an empty database and verifies all
// versioned tables exist and schema_migrations is populated.
func TestMigrateFresh(t *testing.T) {
	db := newTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate fresh: %v", err)
	}

	var count int64
	if err := db.Table("schema_migrations").Count(&count).Error; err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least 1 recorded migration, got %d", count)
	}

	// core tables must exist
	for _, table := range []string{"projects", "sessions", "tasks", "decisions", "blockers", "session_events", "checkpoints", "agent_sessions", "artifacts", "knowledge", "knowledge_fts"} {
		var n int64
		if err := db.Table("sqlite_master").Where("type = 'table' AND name = ?", table).Count(&n).Error; err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if n == 0 {
			t.Fatalf("table %s not created", table)
		}
	}
}

// TestMigrateIdempotent verifies running Migrate twice is a no-op.
func TestMigrateIdempotent(t *testing.T) {
	db := newTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int64
	db.Table("schema_migrations").Count(&count)
	var count2 int64
	db.Table("schema_migrations").Count(&count2)
	if count != count2 {
		t.Fatalf("migration count changed on re-run: %d -> %d", count, count2)
	}
}

// legacySchema builds a database the way the pre-versioned single-file
// migrations.sql did: every table exists, nothing is recorded in
// schema_migrations.
func legacySchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version != "001" {
			continue
		}
		if err := db.Exec(m.sql).Error; err != nil {
			t.Fatalf("build legacy schema: %v", err)
		}
	}
}

// TestMigrateLegacyUpgrade simulates an old database created by the legacy
// single-file schema: tables exist, but schema_migrations has no rows. Migrate
// must record 001 without re-running it, then apply anything newer.
func TestMigrateLegacyUpgrade(t *testing.T) {
	db := newTestDB(t)
	legacySchema(t, db)

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate legacy db: %v", err)
	}

	// 001 must be recorded as already applied, and later versions actually applied
	var count int64
	if err := db.Table("schema_migrations").Where("version = ?", "001").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 001 recorded once for legacy db, got %d", count)
	}

	// the kind column from 002 must exist on the legacy database too
	var kind string
	if err := db.Raw(`SELECT COALESCE((SELECT kind FROM checkpoints LIMIT 1), 'none')`).Scan(&kind).Error; err != nil {
		t.Fatalf("002 was not applied to the legacy database: %v", err)
	}
}

// TestMigratePartialSchemaIsRejected covers a database that holds some of the
// legacy tables but not all — an interrupted first run under the old
// non-transactional schema script. Declaring it migrated skipped 001 and left the
// next migration's ALTER pointing at a table that was never created, producing a
// database that could not be opened. It must fail with an explanation instead.
func TestMigratePartialSchemaIsRejected(t *testing.T) {
	db := newTestDB(t)
	if err := db.Exec(`CREATE TABLE projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, path TEXT NOT NULL UNIQUE)`).Error; err != nil {
		t.Fatal(err)
	}

	err := Migrate(db)
	if err == nil {
		t.Fatal("expected an error for a partially initialized database")
	}
	if !strings.Contains(err.Error(), "partially initialized") {
		t.Errorf("error should explain the partial schema, got: %v", err)
	}
	if !strings.Contains(err.Error(), "checkpoints") {
		t.Errorf("error should name the missing tables, got: %v", err)
	}
}

// TestMigrateAdditive verifies a new migration file is applied to an existing
// migrated database (the core purpose of versioned migrations).
func TestMigrateAdditive(t *testing.T) {
	db := newTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	// simulate a later migration being added: a new table created via raw exec
	// that would be a new .sql file in a real release
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS example_added (id TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create added table: %v", err)
	}

	// re-run must still succeed (idempotent)
	if err := Migrate(db); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

// TestMigrateConcurrentFresh simulates the real boot pattern: an MCP server and
// a CLI hook opening the same never-migrated database at the same time. Every
// caller must succeed, and 001 must be recorded exactly once.
func TestMigrateConcurrentFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	start := make(chan struct{})

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			db, err := Open(path)
			if err != nil {
				errs <- fmt.Errorf("open: %w", err)
				return
			}
			if err := Migrate(db); err != nil {
				errs <- fmt.Errorf("migrate: %w", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("concurrent migrate failed: %v", e)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := db.Table("schema_migrations").Where("version = ?", "001").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected version 001 recorded exactly once, got %d", n)
	}
}

// TestMigrateConcurrentPendingUpgrade covers the upgrade path the versioned
// engine exists for: an already-migrated database gets a new pending version
// while several processes boot at once.
func TestMigrateConcurrentPendingUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	// a genuinely pending step every caller will try to apply at the same time
	pending := []migration{{
		version: "999",
		sql:     `CREATE TABLE concurrent_upgrade_probe (id TEXT PRIMARY KEY)`,
	}}

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			d, err := Open(path)
			if err != nil {
				errs <- fmt.Errorf("open: %w", err)
				return
			}
			if err := migrate(context.Background(), d, pending); err != nil {
				errs <- fmt.Errorf("migrate: %w", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent upgrade failed: %v", e)
	}

	var n int64
	if err := db.Table("schema_migrations").Where("version = ?", "999").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected pending version applied exactly once, got %d", n)
	}
}

// TestMigrate002BackfillsCheckpointKind exercises the versioned engine on a real
// ALTER TABLE — the case the engine was built for — and checks the backfill maps
// the labels the hooks and services were already writing.
func TestMigrate002BackfillsCheckpointKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backfill.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	// migrate to 001 only, then insert rows as the pre-kind code would have
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var first []migration
	for _, m := range ms {
		if m.version == "001" {
			first = append(first, m)
		}
	}
	if len(first) != 1 {
		t.Fatalf("expected migration 001, found %d", len(first))
	}
	if err := migrate(context.Background(), db, first); err != nil {
		t.Fatalf("migrate 001: %v", err)
	}

	if err := db.Exec(`INSERT INTO projects (id, name, path) VALUES ('p1','p','/p')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sessions (id, project_id, title, status, branch) VALUES ('s1','p1','t','active','main')`).Error; err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{
		"c1": "auto",
		"c2": "auto-checkpoint (stale)",
		"c3": "precompact",
		"c4": "handoff",
		"c5": "v0.1.5 released",
		"c6": "",
	}
	for id, label := range labels {
		if err := db.Exec(`INSERT INTO checkpoints (id, session_id, label, snapshot) VALUES (?,?,?,'{}')`,
			id, "s1", label).Error; err != nil {
			t.Fatal(err)
		}
	}

	// now apply everything, which must include 002
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	want := map[string]string{
		"c1": "auto",
		"c2": "auto",
		"c3": "precompact",
		"c4": "handoff",
		"c5": "manual",
		"c6": "manual",
	}
	for id, kind := range want {
		var got string
		if err := db.Raw(`SELECT kind FROM checkpoints WHERE id = ?`, id).Scan(&got).Error; err != nil {
			t.Fatalf("read kind for %s: %v", id, err)
		}
		if got != kind {
			t.Errorf("checkpoint %s (label %q) kind = %q, want %q", id, labels[id], got, kind)
		}
	}
}

// TestTransactionsTakeWriteLockUpFront pins the _txlock=immediate DSN setting. A
// SQLite transaction that starts as a reader cannot wait for the write lock when
// it later needs one — the upgrade fails with SQLITE_BUSY immediately, regardless
// of busy_timeout. Every transaction in this project reads before it writes.
func TestTransactionsTakeWriteLockUpFront(t *testing.T) {
	path := filepath.Join(t.TempDir(), "txlock.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v INTEGER)`).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := db.Exec(`INSERT INTO t (v) VALUES (?)`, i).Error; err != nil {
			t.Fatal(err)
		}
	}

	const n = 6
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := Open(path)
			if err != nil {
				errs <- fmt.Errorf("open: %w", err)
				return
			}
			err = d.Transaction(func(tx *gorm.DB) error {
				var ids []int
				if err := tx.Table("t").Pluck("id", &ids).Error; err != nil {
					return err
				}
				return tx.Exec(`UPDATE t SET v = v + 1`).Error
			})
			if err != nil {
				errs <- fmt.Errorf("read-then-write transaction: %w", err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent read-then-write transaction failed: %v", e)
	}
}

// TestMigrate003NormalizesTimestamps covers the padding of rows written by the
// earlier variable-width encoding, so old and new values compare correctly.
func TestMigrate003NormalizesTimestamps(t *testing.T) {
	db := newTestDB(t)
	legacySchema(t, db)

	if err := db.Exec(`INSERT INTO projects (id, name, path, created_at) VALUES ('p1','p','/p','2026-08-12T10:00:00Z')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sessions (id, project_id, title, status, branch, created_at, updated_at)
		VALUES ('s1','p1','t','active','main','2026-08-12T10:00:00.5Z','2026-08-12T10:00:00Z')`).Error; err != nil {
		t.Fatal(err)
	}
	rows := []struct{ id, at string }{
		{"e1", "2026-08-12T10:00:00Z"},
		{"e2", "2026-08-12T10:00:00.5Z"},
		{"e3", "2026-08-12T10:00:00.123456789Z"},
	}
	for _, r := range rows {
		if err := db.Exec(`INSERT INTO session_events (id, session_id, agent, type, created_at) VALUES (?,?,'claude','file.changed',?)`,
			r.id, "s1", r.at).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// every timestamp must now be fixed width, and ordering must be chronological
	var widths []int
	if err := db.Raw(`SELECT length(created_at) FROM session_events`).Scan(&widths).Error; err != nil {
		t.Fatal(err)
	}
	for _, w := range widths {
		if w != 30 {
			t.Errorf("timestamp width = %d, want 30 (fixed nine-digit fraction)", w)
		}
	}

	var ordered []string
	if err := db.Raw(`SELECT id FROM session_events ORDER BY created_at ASC`).Scan(&ordered).Error; err != nil {
		t.Fatal(err)
	}
	want := []string{"e1", "e3", "e2"} // .000000000 < .123456789 < .500000000
	for i := range want {
		if i >= len(ordered) || ordered[i] != want[i] {
			t.Fatalf("event order = %v, want %v — string order still disagrees with time", ordered, want)
		}
	}

	// a padded value must still parse
	var raw string
	if err := db.Raw(`SELECT created_at FROM projects WHERE id='p1'`).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw != "2026-08-12T10:00:00.000000000Z" {
		t.Errorf("projects.created_at = %q, want padded", raw)
	}
}
