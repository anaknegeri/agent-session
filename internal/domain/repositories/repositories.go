package repositories

import (
	"context"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

type ProjectRepository interface {
	Create(ctx context.Context, project *entities.Project) error
	GetByPath(ctx context.Context, path string) (*entities.Project, error)
	GetByID(ctx context.Context, id string) (*entities.Project, error)
}

type SessionRepository interface {
	Create(ctx context.Context, session *entities.Session) error
	GetByID(ctx context.Context, id string) (*entities.Session, error)
	GetActive(ctx context.Context, projectID string) (*entities.Session, error)
	GetLatest(ctx context.Context, projectID string) (*entities.Session, error)
	Update(ctx context.Context, session *entities.Session) error
	ListByProject(ctx context.Context, projectID string, limit, offset int) ([]*entities.Session, error)
}

type TaskRepository interface {
	Create(ctx context.Context, task *entities.Task) error
	GetByID(ctx context.Context, id string) (*entities.Task, error)
	GetCurrent(ctx context.Context, sessionID string) (*entities.Task, error)
	Update(ctx context.Context, task *entities.Task) error
	ListBySession(ctx context.Context, sessionID string) ([]*entities.Task, error)
}

type DecisionRepository interface {
	Create(ctx context.Context, decision *entities.Decision) error
	ListBySession(ctx context.Context, sessionID string) ([]*entities.Decision, error)
}

type BlockerRepository interface {
	Create(ctx context.Context, blocker *entities.Blocker) error
	ListBySession(ctx context.Context, sessionID string) ([]*entities.Blocker, error)
	ListOpen(ctx context.Context, sessionID string) ([]*entities.Blocker, error)
	Resolve(ctx context.Context, id string, resolvedAt interface{}) error
}

type EventRepository interface {
	Append(ctx context.Context, event *entities.SessionEvent) error
	ListBySession(ctx context.Context, sessionID string, limit int) ([]*entities.SessionEvent, error)
}

type CheckpointRepository interface {
	Create(ctx context.Context, checkpoint *entities.Checkpoint) error
	GetByID(ctx context.Context, id string) (*entities.Checkpoint, error)
	GetLatest(ctx context.Context, sessionID string) (*entities.Checkpoint, error)
	ListBySession(ctx context.Context, sessionID string, limit int) ([]*entities.Checkpoint, error)
	// PruneKind removes the oldest checkpoints of one kind beyond keep, always
	// preserving the session's most recent checkpoint.
	PruneKind(ctx context.Context, sessionID, kind string, keep int) (int, error)
}

type AgentSessionRepository interface {
	Create(ctx context.Context, agentSession *entities.AgentSession) error
	Close(ctx context.Context, id string, endedAt interface{}, checkpointID string) error
	GetLatest(ctx context.Context, sessionID string) (*entities.AgentSession, error)
	ListOpen(ctx context.Context, sessionID string) ([]*entities.AgentSession, error)
	// Resume atomically closes all open agent sessions for the session and
	// creates a new one, guaranteeing exactly one active agent session even
	// under concurrent processes (MCP server + CLI).
	Resume(ctx context.Context, sessionID, agent string) (*entities.AgentSession, error)
}

type ArtifactRepository interface {
	Create(ctx context.Context, artifact *entities.Artifact) error
}

type KnowledgeRepository interface {
	Create(ctx context.Context, knowledge *entities.Knowledge) error
	GetByID(ctx context.Context, id string) (*entities.Knowledge, error)
	ListByKind(ctx context.Context, kind string, limit int) ([]*entities.Knowledge, error)
	Search(ctx context.Context, query string, limit int) ([]*entities.KnowledgeHit, error)
	SearchAny(ctx context.Context, query string, limit int) ([]*entities.KnowledgeHit, error)
	Delete(ctx context.Context, id string) error
	ExistsSource(ctx context.Context, sourceType, sourceID string) (bool, error)
}
