package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/safetext"
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
	sync        *SyncService
	git         ports.GitService
}

func NewContextService(
	store ports.Store,
	checkpoints *CheckpointService,
	renderer ports.ContextRenderer,
	budget ports.ContextBudget,
	sync *SyncService,
	git ports.GitService,
) *ContextService {
	return &ContextService{store: store, checkpoints: checkpoints, renderer: renderer, budget: budget, sync: sync, git: git}
}

// Get renders the session context with progressive loading.
// Auto-syncs file changes and auto-checkpoints when stale before rendering.
func (s *ContextService) Get(ctx context.Context, sessionID, depth string) (string, error) {
	// Fetch git status once and thread it through sync/staleness/nudges below —
	// each used to shell out to git separately, tripling the cost of every call.
	status, haveStatus := s.gitStatus(ctx, sessionID)
	if haveStatus && s.sync != nil {
		s.sync.SyncFileChanges(ctx, sessionID, "sync", status)
		if s.sync.IsStale(ctx, sessionID, status) {
			s.checkpoints.CreateKind(ctx, sessionID, entities.CheckpointKindAuto, "auto-checkpoint (stale)", "", "sync")
		}
	}

	snapshot, err := s.checkpoints.BuildSnapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}

	renderBudget := s.budget
	if depth == ContextDepthFull {
		renderBudget = ports.ContextBudget{}
	} else if depth == ContextDepthRecent {
		renderBudget.MaxTotalChars = 0
	}

	// add nudges to snapshot
	if depth == ContextDepthSummary {
		s.addNudges(ctx, snapshot, status, haveStatus)
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
				text += fmt.Sprintf("- %s [%s]\n", safetext.SingleLine(e.Type), safetext.SingleLine(e.Agent))
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
	text += "\n## Relevant memory " + string(entities.TrustAgentNote.Label()) + "\n"
	for _, h := range hits {
		// promoted from agent-authored content, so it carries the same risk
		text += fmt.Sprintf("- %s\n", truncateString(safetext.SingleLine(h.Content), s.budget.MaxItemChars))
	}
	return text
}

// addNudges warns about staleness so the agent knows when to checkpoint or
// record events. Appended to snapshot.Nudges, rendered by the markdown renderer.
func (s *ContextService) addNudges(ctx context.Context, snapshot *entities.Snapshot, status ports.WorkspaceStatus, haveStatus bool) {
	if s.sync == nil {
		return
	}

	// stale checkpoint warning
	minutes := s.sync.MinutesSinceCheckpoint(ctx, snapshot.Session.ID)
	if minutes > 30 {
		snapshot.Nudges = append(snapshot.Nudges, fmt.Sprintf("⚠ Last checkpoint was %d minutes ago — run session.checkpoint", minutes))
	}

	// unrecorded file changes
	if haveStatus {
		if unrecorded := s.sync.UnrecordedFileCount(ctx, snapshot.Session.ID, status); unrecorded > 0 {
			snapshot.Nudges = append(snapshot.Nudges, fmt.Sprintf("⚠ %d changed files not yet recorded — context.get will auto-record them", unrecorded))
		}
	}

	// open blockers with no resolution
	for _, b := range snapshot.Blockers {
		if b.Status == entities.BlockerStatusOpen {
			snapshot.Nudges = append(snapshot.Nudges, fmt.Sprintf("⚠ Open blocker: %s", truncateString(safetext.SingleLine(b.Description), 60)))
		}
	}
}

// projectPath resolves the project path for a session (needed for git lookups).
func (s *ContextService) projectPath(ctx context.Context, sessionID string) (string, error) {
	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return "", err
	}
	project, err := s.store.Projects().GetByID(ctx, session.ProjectID)
	if err != nil {
		return "", err
	}
	return project.Path, nil
}

// gitStatus fetches workspace status once per Get() call. haveStatus is false
// when git is unavailable or the project path can't be resolved, in which
// case callers skip sync/staleness/nudge logic entirely rather than guessing.
func (s *ContextService) gitStatus(ctx context.Context, sessionID string) (ports.WorkspaceStatus, bool) {
	if s.git == nil {
		return ports.WorkspaceStatus{}, false
	}
	project, err := s.projectPath(ctx, sessionID)
	if err != nil {
		return ports.WorkspaceStatus{}, false
	}
	status, err := s.git.Status(ctx, project)
	if err != nil {
		return ports.WorkspaceStatus{}, false
	}
	return status, true
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
