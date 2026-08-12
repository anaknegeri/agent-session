package database

import (
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

// appliedVersions returns the set of migration versions already recorded.
func appliedVersions(db *gorm.DB) (map[string]bool, error) {
	// ensure the tracking table exists
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`).Error; err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	var rows []struct {
		Version string
	}
	if err := db.Table("schema_migrations").Select("version").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}

	applied := make(map[string]bool, len(rows))
	for _, r := range rows {
		applied[r.Version] = true
	}
	return applied, nil
}

// legacySchemaDetected reports whether tables were created by the pre-versioned
// migrations.sql (CREATE TABLE IF NOT EXISTS) that never recorded a version.
// When true, the engine records the legacy schema as applied without running it.
func legacySchemaDetected(db *gorm.DB) bool {
	var count int64
	err := db.Table("projects").Count(&count).Error
	return err == nil
}

// migrate applies pending versioned migrations. A database created by the old
// single-file migrations.sql (tables exist but no schema_migrations rows) is
// treated as already at version 001.
func migrate(db *gorm.DB) error {
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	// Backward compatibility: an existing DB from the pre-versioned era has the
	// tables but no recorded versions. Mark 001 as applied without re-running.
	if len(applied) == 0 && legacySchemaDetected(db) {
		if err := db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES ('001')`).Error; err != nil {
			return fmt.Errorf("record legacy schema: %w", err)
		}
		applied["001"] = true
	}

	ms, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
	}
	return nil
}

// applyMigration runs one migration inside a transaction and records it.
func applyMigration(db *gorm.DB, m migration) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(m.sql).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, m.version).Error
	})
}
