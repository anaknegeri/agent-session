package services_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
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
	artifacts := app.NewArtifactService(f.app.Store)
	checkpoints := app.NewCheckpointService(f.app.Store, git, artifacts, log, config.RetentionConfig{})
	sync := app.NewSyncService(f.app.Store, git, artifacts, log)
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

// TestCheckpointAndContextWriteReadGitOnce pins the other half of the pipeline.
// Creating a checkpoint and rewriting context.md both need the workspace state,
// and the two use cases used to read it separately — Checkpoint.Create built a
// snapshot from one `git status`, then WriteContextMD went through Get, read git
// again, rebuilt the same snapshot and re-ran the staleness check the checkpoint
// had just satisfied. That is on every Stop hook and every session.record.
func TestCheckpointAndContextWriteReadGitOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session
	writeFile(t, f.dir, "server.go", "package oauth\n\n// changed\n")

	git := &countingGit{GitService: gitinfra.NewRunner()}
	log := logger.New("error")
	artifacts := app.NewArtifactService(f.app.Store)
	checkpoints := app.NewCheckpointService(f.app.Store, git, artifacts, log, config.RetentionConfig{})
	sync := app.NewSyncService(f.app.Store, git, artifacts, log)
	contexts := app.NewContextService(
		f.app.Store, checkpoints, mdcontext.NewRenderer(),
		ports.DefaultContextBudget(), sync, git,
	)

	ws := contexts.ReadWorkspace(ctx, session.ID)
	snapshot, err := contexts.SnapshotFrom(ctx, session.ID, ws)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := checkpoints.CreateFromSnapshot(ctx, session.ID,
		entities.CheckpointKindManual, "manual", "next", "cli", snapshot); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if _, err := contexts.WriteContextMDFrom(ctx, f.dir, session.ID, snapshot, ws); err != nil {
		t.Fatalf("write context.md: %v", err)
	}

	if n := git.statusCalls.Load(); n != 1 {
		t.Errorf("checkpoint + context.md called git.Status %d times, want 1", n)
	}

	// The rendered file has to be the real thing, not an empty win: a reuse that
	// dropped the workspace section would also read git once.
	data, err := os.ReadFile(filepath.Join(f.dir, ".agent", "context", "current.md"))
	if err != nil {
		t.Fatalf("read context.md: %v", err)
	}
	rendered := string(data)
	if !strings.Contains(rendered, "**Branch:**") {
		t.Errorf("context.md lost its workspace line:\n%s", rendered)
	}
	if !strings.Contains(rendered, "server.go") {
		t.Errorf("context.md does not mention the changed file:\n%s", rendered)
	}
}
