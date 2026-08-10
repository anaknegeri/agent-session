package database

import (
	"fmt"

	"github.com/agent-session/agent-session/internal/application/ports"
	"github.com/agent-session/agent-session/internal/infrastructure/database/store"
)

// OpenStore opens and migrates a SQLite database at path, returning a ports.Store.
// path can be ":memory:" for tests.
func OpenStore(path string) (ports.Store, error) {
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return store.NewSQLiteStore(db)
}
