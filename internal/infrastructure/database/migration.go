package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is a single versioned schema step.
type migration struct {
	version string
	sql     string
}

// loadMigrations reads all migrations/*.sql files, sorted by version.
func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		version := strings.SplitN(e.Name(), "_", 2)[0]
		ms = append(ms, migration{version: version, sql: string(data)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}

// appliedVersions returns the set of migration versions already recorded,
// creating the tracking table when it does not exist yet.
func appliedVersions(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations: %w", err)
	}
	return applied, nil
}

// legacySchemaDetected reports whether tables were created by the pre-versioned
// migrations.sql (CREATE TABLE IF NOT EXISTS) that never recorded a version.
// When true, the engine records the legacy schema as applied without running it.
func legacySchemaDetected(ctx context.Context, conn *sql.Conn) (bool, error) {
	var n int
	err := conn.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'projects'`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("detect legacy schema: %w", err)
	}
	return n > 0, nil
}

// migrate applies pending versioned migrations. A database created by the old
// single-file migrations.sql (tables exist but no schema_migrations rows) is
// treated as already at version 001.
//
// Reading the applied set and applying the pending steps happen inside one
// BEGIN IMMEDIATE transaction. The write lock is taken up front so that
// concurrent processes (the MCP server and a CLI hook booting together) queue
// on it instead of both concluding a version is pending and racing to apply it.
func migrate(ctx context.Context, db *gorm.DB, ms []migration) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	// Backward compatibility: an existing DB from the pre-versioned era has the
	// tables but no recorded versions. Mark 001 as applied without re-running.
	if len(applied) == 0 {
		legacy, err := legacySchemaDetected(ctx, conn)
		if err != nil {
			return err
		}
		if legacy {
			if _, err := conn.ExecContext(ctx,
				`INSERT OR IGNORE INTO schema_migrations (version) VALUES ('001')`); err != nil {
				return fmt.Errorf("record legacy schema: %w", err)
			}
			applied["001"] = true
		}
	}

	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if err := applyMigration(ctx, conn, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	committed = true
	return nil
}

// applyMigration runs one migration and records its version. The caller owns the
// surrounding transaction.
func applyMigration(ctx context.Context, conn *sql.Conn, m migration) error {
	if _, err := conn.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return nil
}
