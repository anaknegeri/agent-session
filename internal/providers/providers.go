package providers

import (
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	mdc "github.com/anaknegeri/agent-session/internal/infrastructure/context"
	gitrunner "github.com/anaknegeri/agent-session/internal/infrastructure/git"
)

func ProvideInitService(store ports.Store, git ports.GitService, logger *slog.Logger) *app.InitService {
	return app.NewInitService(store, git, logger)
}

func ProvideSessionService(store ports.Store, git ports.GitService, logger *slog.Logger) *app.SessionService {
	return app.NewSessionService(store, git, logger)
}

func ProvideTaskService(store ports.Store, logger *slog.Logger) *app.TaskService {
	return app.NewTaskService(store, logger)
}

func ProvideDecisionService(store ports.Store, logger *slog.Logger) *app.DecisionService {
	return app.NewDecisionService(store, logger)
}

func ProvideEventService(store ports.Store) *app.EventService {
	return app.NewEventService(store)
}

func ProvideWorkspaceService(git ports.GitService) *app.WorkspaceService {
	return app.NewWorkspaceService(git)
}

func ProvideCheckpointService(store ports.Store, git ports.GitService, logger *slog.Logger) *app.CheckpointService {
	return app.NewCheckpointService(store, git, logger)
}

func ProvideContextService(store ports.Store, checkpoints *app.CheckpointService, renderer ports.ContextRenderer, budget ports.ContextBudget) *app.ContextService {
	return app.NewContextService(store, checkpoints, renderer, budget)
}

func ProvideRenderer() ports.ContextRenderer {
	return mdc.NewRenderer()
}

func ProvideGitService() ports.GitService {
	return gitrunner.NewRunner()
}

func ProvideHandoffService(store ports.Store, checkpoints *app.CheckpointService, renderer ports.ContextRenderer, logger *slog.Logger) *app.HandoffService {
	return app.NewHandoffService(store, checkpoints, renderer, logger)
}

func ProvideArtifactService(store ports.Store) *app.ArtifactService {
	return app.NewArtifactService(store)
}

func ProvideMemoryService(store ports.Store, logger *slog.Logger) *app.MemoryService {
	return app.NewMemoryService(store, logger)
}

func ProvideExportService(store ports.Store) *app.ExportService {
	return app.NewExportService(store)
}

func ProvideApp(
	init *app.InitService,
	session *app.SessionService,
	task *app.TaskService,
	decision *app.DecisionService,
	event *app.EventService,
	workspace *app.WorkspaceService,
	checkpoint *app.CheckpointService,
	contextSvc *app.ContextService,
	handoff *app.HandoffService,
	artifact *app.ArtifactService,
	memory *app.MemoryService,
	exportSvc *app.ExportService,
	store ports.Store,
) *app.App {
	return &app.App{
		Init:       init,
		Session:    session,
		Task:       task,
		Decision:   decision,
		Event:      event,
		Workspace:  workspace,
		Checkpoint: checkpoint,
		Context:    contextSvc,
		Handoff:    handoff,
		Artifact:   artifact,
		Memory:     memory,
		Export:     exportSvc,
		Store:      store,
	}
}
