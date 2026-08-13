package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type sqliteStore struct {
	db *gorm.DB
	// scoped marks a store handed to a Tx callback: it borrows the transaction's
	// connection and must not close the pool underneath it.
	scoped bool

	projects      repositories.ProjectRepository
	sessions      repositories.SessionRepository
	tasks         repositories.TaskRepository
	decisions     repositories.DecisionRepository
	blockers      repositories.BlockerRepository
	events        repositories.EventRepository
	checkpoints   repositories.CheckpointRepository
	agentSessions repositories.AgentSessionRepository
	artifacts     repositories.ArtifactRepository
	knowledge     repositories.KnowledgeRepository
}

func NewSQLiteStore(db *gorm.DB) (*sqliteStore, error) {
	return newSQLiteStore(db, false), nil
}

func newSQLiteStore(db *gorm.DB, scoped bool) *sqliteStore {
	s := &sqliteStore{db: db, scoped: scoped}
	s.projects = &projectStore{db: db}
	s.sessions = &sessionStore{db: db}
	s.tasks = &taskStore{db: db}
	s.decisions = &decisionStore{db: db}
	s.blockers = &blockerStore{db: db}
	s.events = &eventStore{db: db}
	s.checkpoints = &checkpointStore{db: db}
	s.agentSessions = &agentSessionStore{db: db}
	s.artifacts = &artifactStore{db: db}
	s.knowledge = &knowledgeStore{db: db}
	return s
}

func (s *sqliteStore) Projects() repositories.ProjectRepository           { return s.projects }
func (s *sqliteStore) Sessions() repositories.SessionRepository           { return s.sessions }
func (s *sqliteStore) Tasks() repositories.TaskRepository                 { return s.tasks }
func (s *sqliteStore) Decisions() repositories.DecisionRepository         { return s.decisions }
func (s *sqliteStore) Blockers() repositories.BlockerRepository           { return s.blockers }
func (s *sqliteStore) Events() repositories.EventRepository               { return s.events }
func (s *sqliteStore) Checkpoints() repositories.CheckpointRepository     { return s.checkpoints }
func (s *sqliteStore) AgentSessions() repositories.AgentSessionRepository { return s.agentSessions }
func (s *sqliteStore) Artifacts() repositories.ArtifactRepository         { return s.artifacts }
func (s *sqliteStore) Knowledge() repositories.KnowledgeRepository        { return s.knowledge }

// Tx binds every repository to a single GORM transaction for the duration of fn.
// GORM turns a nested Transaction call into a SAVEPOINT, so repository methods
// that already open their own (StartExclusive, AgentSessions.Resume) keep working
// unchanged and commit with the outer boundary.
func (s *sqliteStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(newSQLiteStore(tx, true))
	})
}

func (s *sqliteStore) Close() error {
	if s.scoped {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	return sqlDB.Close()
}
