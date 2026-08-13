package ports

import (
	"context"

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
	Knowledge() repositories.KnowledgeRepository

	// Tx runs fn against a Store whose repositories all share one transaction, so
	// a use case that writes several rows commits them together or not at all.
	// Returning an error rolls every write back. A repository that opens its own
	// transaction inside fn joins this one instead of starting a second.
	//
	// fn must not do slow work: the transaction holds the write lock, and other
	// agents on the same project are blocked meanwhile. Read git, render text and
	// marshal payloads before entering it.
	Tx(ctx context.Context, fn func(Store) error) error

	Close() error
}
