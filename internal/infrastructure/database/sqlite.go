package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// openAttempts bounds the retries for the exclusive lock SQLite needs when it
// switches a fresh database into WAL mode; a co-starting process (MCP server vs
// CLI hook) can hold it briefly.
const openAttempts = 10

// openRetryDelay is the backoff base between open attempts.
const openRetryDelay = 25 * time.Millisecond

// Open opens (and creates if needed) a SQLite database at path.
// Use ":memory:" for tests.
func Open(path string) (*gorm.DB, error) {
	// busy_timeout comes first so it is already in effect when the journal_mode
	// change below contends with another connection.
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}

	cfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	var lastErr error
	for attempt := 0; attempt < openAttempts; attempt++ {
		db, err := gorm.Open(sqlite.Open(dsn), cfg)
		if err == nil {
			// An in-memory database is private to its connection, so a pool of
			// more than one would hand out empty databases at random.
			if path == ":memory:" {
				sqlDB, derr := db.DB()
				if derr != nil {
					return nil, fmt.Errorf("open sqlite %s: %w", path, derr)
				}
				sqlDB.SetMaxOpenConns(1)
			}
			return db, nil
		}
		lastErr = err
		if !isLocked(err) {
			break
		}
		time.Sleep(time.Duration(attempt+1) * openRetryDelay)
	}
	return nil, fmt.Errorf("open sqlite %s: %w", path, lastErr)
}

// isLocked reports whether err is SQLite's busy/locked condition, which is
// transient and worth retrying.
func isLocked(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "SQLITE_LOCKED")
}

// Migrate applies pending versioned migrations (migrations/*.sql), recorded in
// schema_migrations. Databases created by the legacy single-file schema are
// detected and marked as migrated without re-running. Safe to call concurrently
// from several processes.
func Migrate(db *gorm.DB) error {
	ms, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if err := migrate(context.Background(), db, ms); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// Ping verifies the connection is alive.
func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}
