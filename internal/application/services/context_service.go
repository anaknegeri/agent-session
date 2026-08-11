package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
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
	budget      ports.ContextBudget
}

func NewContextService(
	store ports.Store,
	checkpoints *CheckpointService,
	renderer ports.ContextRenderer,
	budget ports.ContextBudget,
) *ContextService {
	return &ContextService{store: store, checkpoints: checkpoints, renderer: renderer, budget: budget}
}

// Get renders the session context with progressive loading (PRD §25).
// summary: current task + decisions + blockers + git + recent events + relevant memory,
// bounded by the context budget to save tokens.
func (s *ContextService) Get(ctx context.Context, sessionID, depth string) (string, error) {
	snapshot, err := s.checkpoints.BuildSnapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}

	renderBudget := s.budget
	if depth == ContextDepthFull {
		// explicit full detail: no list truncation, no clamp
		renderBudget = ports.ContextBudget{}
	} else if depth == ContextDepthRecent {
		// bounded lists but never hard-clamped
		renderBudget.MaxTotalChars = 0
	}

	text, err := s.renderer.RenderContext(snapshot, renderBudget)
	if err != nil {
		return "", err
	}

	if depth == ContextDepthFull || depth == ContextDepthRecent {
		events, err := s.store.Events().ListBySession(ctx, sessionID, eventLimit(s.budget.MaxEvents, depth))
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

	if depth == ContextDepthSummary && s.budget.InjectMemory {
		text = s.injectMemory(ctx, text, snapshot)
	}

	// Hard clamp only applies to the cheap summary view. Full/recent depth is
	// explicitly requested detail and is never silently cut.
	if depth == ContextDepthSummary {
		text = clamp(text, s.budget.MaxTotalChars)
	}
	return text, nil
}

// injectMemory appends top-k related knowledge retrieved by OR full-text search
// of the current task title (local-first, no embeddings).
func (s *ContextService) injectMemory(ctx context.Context, text string, snapshot *entities.Snapshot) string {
	if snapshot.Task.Title == "" || s.budget.MaxMemory <= 0 {
		return text
	}
	hits, err := s.store.Knowledge().SearchAny(ctx, snapshot.Task.Title, s.budget.MaxMemory)
	if err != nil || len(hits) == 0 {
		return text
	}
	text += "\n## Relevant memory\n"
	for _, h := range hits {
		text += fmt.Sprintf("- %s\n", truncateString(h.Content, s.budget.MaxItemChars))
	}
	return text
}

func eventLimit(max int, depth string) int {
	if max <= 0 {
		return 0
	}
	if depth == ContextDepthFull {
		return max * 2
	}
	return max
}

// clamp trims text to at most maxChars, keeping a trailing note that the full
// state is still available on demand.
func clamp(text string, maxChars int) string {
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxChars]) + "\n… (summary clamped — call `context.get depth=full` for the complete state)"
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
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
