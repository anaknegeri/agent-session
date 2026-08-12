//go:build wireinject
// +build wireinject

package wire

import (
	"log/slog"

	"github.com/google/wire"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/providers"
)

var appSet = wire.NewSet(
	providers.ProvideInitService,
	providers.ProvideSessionService,
	providers.ProvideTaskService,
	providers.ProvideDecisionService,
	providers.ProvideEventService,
	providers.ProvideWorkspaceService,
	providers.ProvideCheckpointService,
	providers.ProvideContextService,
	providers.ProvideHandoffService,
	providers.ProvideArtifactService,
	providers.ProvideMemoryService,
	providers.ProvideExportService,
	providers.ProvideSyncService,
	providers.ProvideRenderer,
	providers.ProvideGitService,
	providers.ProvideApp,
)

// InitializeApp wires the application with a runtime store.
func InitializeApp(store ports.Store, logger *slog.Logger, budget ports.ContextBudget) (*app.App, error) {
	wire.Build(appSet)
	return &app.App{}, nil
}
