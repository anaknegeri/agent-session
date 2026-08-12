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

// legacyTables are every table migration 001 creates. A database from the
// pre-versioned era has all of them and no schema_migrations rows.
var legacyTables = []string{
	"projects", "sessions", "tasks", "decisions", "blockers",
	"session_events", "checkpoints", "agent_sessions", "artifacts",
	"knowledge", "knowledge_fts",
}

// legacySchemaDetected reports whether the tables were created by the
// pre-versioned migrations.sql, which never recorded a version. When true, the
// engine records 001 as applied without re-running it.
//
// It requires every table, not just one. Checking a single table meant a
// partially created database was declared migrated, so 001 was skipped and the
// next migration's ALTER hit a table that was never created — leaving a database
// that could not be opened at all. A partial schema is reported as an error
// instead, since 001 can no longer be re-run over existing tables.
func legacySchemaDetected(ctx context.Context, conn *sql.Conn) (bool, error) {
	present := make([]string, 0, len(legacyTables))
	missing := make([]string, 0, len(legacyTables))
	for _, table := range legacyTables {
		var n int
		err := conn.QueryRowContext(ctx,
			`SELECT count(*) FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, table).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("detect legacy schema: %w", err)
		}
		if n > 0 {
			present = append(present, table)
		} else {
			missing = append(missing, table)
		}
	}

	switch {
	case len(present) == 0:
		return false, nil
	case len(missing) == 0:
		return true, nil
	default:
		return false, fmt.Errorf(
			"database is partially initialized: %d of %d tables exist (missing: %s). "+
				"Restore a backup, or remove .agent/session.db to start a fresh session store",
			len(present), len(legacyTables), strings.Join(missing, ", "))
	}
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
