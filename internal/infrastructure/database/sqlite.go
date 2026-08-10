package database

import (
	"context"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open opens (and creates if needed) a SQLite database at path.
// Use ":memory:" for tests.
func Open(path string) (*gorm.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	return db, nil
}

// Migrate applies the embedded schema.
func Migrate(db *gorm.DB) error {
	if err := db.Exec(migrationsSQL).Error; err != nil {
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
