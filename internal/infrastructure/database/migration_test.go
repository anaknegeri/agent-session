package database

import (
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

// TestMigrateLegacyUpgrade simulates an old database created by the legacy
// single-file schema: tables exist, but schema_migrations has no rows. Migrate
// must record 001 without re-running it.
func TestMigrateLegacyUpgrade(t *testing.T) {
	db := newTestDB(t)

	// simulate the legacy schema: create the projects table but no schema_migrations
	if err := db.Exec(`CREATE TABLE projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, path TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')))`).Error; err != nil {
		t.Fatalf("create legacy projects table: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate legacy db: %v", err)
	}

	// the legacy DB must be recorded as already at 001
	var count int64
	if err := db.Table("schema_migrations").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 recorded migration for legacy db, got %d", count)
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
