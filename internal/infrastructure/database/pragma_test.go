package database

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

// TestPragmasActive verifies WAL and foreign_keys are actually enabled on the
// connection, not just present in the DSN string in sqlite.go — a typo or a
// driver quirk could silently ignore either, and Open's retry loop and every
// ON DELETE CASCADE in the schema depend on both being active.
func TestPragmasActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pragma.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}

	var journalMode string
	if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := sqlDB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

// TestCascadeDeleteRemovesArtifacts pins the artifacts -> sessions ON DELETE
// CASCADE declared in 001_initial.sql: deleting a session must remove its
// artifacts too, without any application-layer code doing it explicitly. This
// only proves anything because foreign_keys is really on — SQLite silently
// ignores every ON DELETE CASCADE clause in the schema when it is off, which
// is exactly the failure mode TestPragmasActive exists to catch separately.
func TestCascadeDeleteRemovesArtifacts(t *testing.T) {
	db := newTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`INSERT INTO projects (id, name, path) VALUES ('proj1', 'p', '/tmp/p')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sessions (id, project_id, title, branch) VALUES ('sess1', 'proj1', 't', 'main')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO artifacts (id, session_id, kind) VALUES ('art1', 'sess1', 'log')`).Error; err != nil {
		t.Fatal(err)
	}

	var before int64
	if err := db.Raw(`SELECT COUNT(*) FROM artifacts WHERE id = 'art1'`).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("setup: expected the artifact row to exist before delete, got %d", before)
	}

	if err := db.Exec(`DELETE FROM sessions WHERE id = 'sess1'`).Error; err != nil {
		t.Fatal(err)
	}

	var after int64
	if err := db.Raw(`SELECT COUNT(*) FROM artifacts WHERE id = 'art1'`).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Errorf("expected the artifact removed by ON DELETE CASCADE, got %d row(s) still present", after)
	}
}

// TestIsLocked pins the exact substrings isLocked treats as a transient
// busy/locked condition worth retrying in Open's retry loop. A future refactor
// that changes these substrings should fail this test instead of silently
// turning every busy/locked error into an immediate, non-retried failure.
func TestIsLocked(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"sqlite busy substring", errors.New("database table is locked: SQLITE_BUSY"), true},
		{"database is locked phrase", errors.New("database is locked"), true},
		{"sqlite locked substring", errors.New("SQLITE_LOCKED"), true},
		{"substring survives wrapping", fmt.Errorf("open sqlite x: %w", errors.New("disk I/O error: SQLITE_BUSY (5)")), true},
		{"unrelated sqlite error", errors.New("no such table: sessions"), false},
		{"empty message", errors.New(""), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isLocked(c.err); got != c.want {
				t.Errorf("isLocked(%q) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
