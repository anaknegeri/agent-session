package ports

import (
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

// Store abstracts the persistence layer (PRD §17). SQLiteStore today,
// PostgresStore can be added without touching services.
type Store interface {
	Projects() repositories.ProjectRepository
	Sessions() repositories.SessionRepository
	Tasks() repositories.TaskRepository
	Decisions() repositories.DecisionRepository
	Blockers() repositories.BlockerRepository
	Events() repositories.EventRepository
	Checkpoints() repositories.CheckpointRepository
	AgentSessions() repositories.AgentSessionRepository
	Artifacts() repositories.ArtifactRepository
	Close() error
}
