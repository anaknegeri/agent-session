package store

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type sqliteStore struct {
	db *gorm.DB

	projects      repositories.ProjectRepository
	sessions      repositories.SessionRepository
	tasks         repositories.TaskRepository
	decisions     repositories.DecisionRepository
	blockers      repositories.BlockerRepository
	events        repositories.EventRepository
	checkpoints   repositories.CheckpointRepository
	agentSessions repositories.AgentSessionRepository
	artifacts     repositories.ArtifactRepository
}

func NewSQLiteStore(db *gorm.DB) (*sqliteStore, error) {
	s := &sqliteStore{db: db}
	s.projects = &projectStore{db: db}
	s.sessions = &sessionStore{db: db}
	s.tasks = &taskStore{db: db}
	s.decisions = &decisionStore{db: db}
	s.blockers = &blockerStore{db: db}
	s.events = &eventStore{db: db}
	s.checkpoints = &checkpointStore{db: db}
	s.agentSessions = &agentSessionStore{db: db}
	s.artifacts = &artifactStore{db: db}
	return s, nil
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

func (s *sqliteStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	return sqlDB.Close()
}
