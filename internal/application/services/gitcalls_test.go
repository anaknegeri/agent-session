package services_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/config"
	mdcontext "github.com/anaknegeri/agent-session/internal/infrastructure/context"
	gitinfra "github.com/anaknegeri/agent-session/internal/infrastructure/git"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// countingGit wraps a GitService and counts Status calls, so the number of times
// a use case shells out to git is an assertion rather than a claim.
type countingGit struct {
	ports.GitService
	statusCalls atomic.Int64
}

func (g *countingGit) Status(ctx context.Context, dir string) (ports.WorkspaceStatus, error) {
	g.statusCalls.Add(1)
	return g.GitService.Status(ctx, dir)
}

// TestContextGetReadsGitOnce pins the cost of context.get: the workspace status is
// read once and threaded through sync, the snapshot, the staleness check and the
// nudges. It used to be read again by BuildSnapshot, and a third time by the
// snapshot the stale auto-checkpoint built for itself.
func TestContextGetReadsGitOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	// dirty the tree so the staleness path is live, not skipped
	writeFile(t, f.dir, "server.go", "package oauth\n\n// changed\n")

	git := &countingGit{GitService: gitinfra.NewRunner()}
	log := logger.New("error")
	checkpoints := app.NewCheckpointService(f.app.Store, git, log, config.RetentionConfig{})
	sync := app.NewSyncService(f.app.Store, git, app.NewArtifactService(f.app.Store), log)
	contexts := app.NewContextService(
		f.app.Store, checkpoints, mdcontext.NewRenderer(),
		ports.DefaultContextBudget(), sync, git,
	)

	if _, err := contexts.Get(ctx, session.ID, app.ContextDepthSummary); err != nil {
		t.Fatalf("context.get: %v", err)
	}

	if n := git.statusCalls.Load(); n != 1 {
		t.Errorf("context.get called git.Status %d times, want 1", n)
	}
}
