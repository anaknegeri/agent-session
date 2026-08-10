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

func (s *WorkspaceService) Diff(ctx context.Context, dir string) (string, error) {
	return s.git.Diff(ctx, dir)
}
