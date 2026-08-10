package services

import (
	"context"

	"github.com/agent-session/agent-session/internal/application/ports"
)

// App bundles all application services for entrypoints (CLI, MCP).
type App struct {
	Init       *InitService
	Session    *SessionService
	Task       *TaskService
	Decision   *DecisionService
	Event      *EventService
	Workspace  *WorkspaceService
	Checkpoint *CheckpointService
	Context    *ContextService
	Handoff    *HandoffService
	Artifact   *ArtifactService
	Store      ports.Store
}

// ResolveProjectID returns the project ID for a registered path.
func (a *App) ResolveProjectID(ctx context.Context, path string) (string, error) {
	project, err := a.Store.Projects().GetByPath(ctx, path)
	if err != nil {
		return "", err
	}
	return project.ID, nil
}
