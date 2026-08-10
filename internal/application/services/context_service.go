package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agent-session/agent-session/internal/application/ports"
	"github.com/agent-session/agent-session/internal/config"
)

const (
	ContextDepthSummary = "summary"
	ContextDepthRecent  = "recent"
	ContextDepthFull    = "full"
)

type ContextService struct {
	store       ports.Store
	checkpoints *CheckpointService
	renderer    ports.ContextRenderer
}

func NewContextService(
	store ports.Store,
	checkpoints *CheckpointService,
	renderer ports.ContextRenderer,
) *ContextService {
	return &ContextService{store: store, checkpoints: checkpoints, renderer: renderer}
}

// Get renders the session context with progressive loading (PRD §25).
// summary: current task + latest checkpoint + decisions + blockers + git + recent events.
func (s *ContextService) Get(ctx context.Context, sessionID, depth string) (string, error) {
	snapshot, err := s.checkpoints.BuildSnapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}

	text, err := s.renderer.RenderContext(snapshot)
	if err != nil {
		return "", err
	}

	if depth == ContextDepthFull || depth == ContextDepthRecent {
		events, err := s.store.Events().ListBySession(ctx, sessionID, 20)
		if err != nil {
			return "", err
		}
		if len(events) > 0 {
			text += "\n## Recent events\n"
			for _, e := range events {
				text += fmt.Sprintf("- %s [%s]\n", e.Type, e.Agent)
			}
		}
	}

	return text, nil
}

// WriteContextMD writes context.md into the .agent dir (human-readable).
func (s *ContextService) WriteContextMD(ctx context.Context, root, sessionID string) (string, error) {
	text, err := s.Get(ctx, sessionID, ContextDepthSummary)
	if err != nil {
		return "", err
	}

	dir := filepath.Join(root, config.DirName, config.ContextDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create context dir: %w", err)
	}
	path := filepath.Join(dir, "current.md")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", fmt.Errorf("write context.md: %w", err)
	}
	return path, nil
}
