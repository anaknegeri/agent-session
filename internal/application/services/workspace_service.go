package services

import (
	"context"

	"github.com/anaknegeri/agent-session/internal/application/ports"
)

type WorkspaceService struct {
	git ports.GitService
}

func NewWorkspaceService(git ports.GitService) *WorkspaceService {
	return &WorkspaceService{git: git}
}

func (s *WorkspaceService) Status(ctx context.Context, dir string) (ports.WorkspaceStatus, error) {
	return s.git.Status(ctx, dir)
}

func (s *WorkspaceService) DetectGit(ctx context.Context, dir string) (bool, error) {
	return s.git.Detect(ctx, dir)
}

// Diff returns the git diff at the requested scope. An unrecognised scope falls
// back to the full diff: only "stat" narrows the output, so a typo must not
// silently hand back less than the caller asked for.
func (s *WorkspaceService) Diff(ctx context.Context, dir, scope string) (string, error) {
	gitScope := ports.DiffScopeFull
	if scope == string(ports.DiffScopeStat) {
		gitScope = ports.DiffScopeStat
	}
	return s.git.Diff(ctx, dir, gitScope)
}
