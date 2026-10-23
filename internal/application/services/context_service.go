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
	// Fetch git status once and thread it through sync/staleness/snapshot/nudges
	// below — each used to shell out to git separately, so a single context.get on
	// a stale session ran git three times over.
	status, haveStatus := s.gitStatus(ctx, sessionID)
	if haveStatus && s.sync != nil {
		s.sync.SyncFileChanges(ctx, sessionID, "sync", status)
	}

	snapshot, err := s.checkpoints.BuildSnapshotWith(ctx, sessionID, status, haveStatus)
	if err != nil {
		return "", err
	}

	// The auto-checkpoint stores this snapshot rather than building its own. It has
	// to happen before the nudges are added: those are advice for whoever is reading
	// now, not state worth preserving.
	if haveStatus && s.sync != nil && s.sync.IsStale(ctx, sessionID, status) {
		s.checkpoints.CreateFromSnapshot(ctx, sessionID, entities.CheckpointKindAuto, "auto-checkpoint (stale)", "", "sync", snapshot)
	}

	return s.render(ctx, sessionID, depth, snapshot, status, haveStatus)
}

// Read renders the same context as Get without writing anything: no file-change
// sync and no auto-checkpoint.
//
// It exists because "reads the session" and "records that it was read" are
// separate promises. A client that gates tool calls on the MCP readOnly hint —
// Codex under `approval: never` does — must be able to trust that hint, so a tool
// annotated read-only has to route here rather than through Get.
func (s *ContextService) Read(ctx context.Context, sessionID, depth string) (string, error) {
	status, haveStatus := s.gitStatus(ctx, sessionID)
	snapshot, err := s.checkpoints.BuildSnapshotWith(ctx, sessionID, status, haveStatus)
	if err != nil {
		return "", err
	}
	return s.render(ctx, sessionID, depth, snapshot, status, haveStatus)
}

func (s *ContextService) render(
	ctx context.Context,
	sessionID, depth string,
	snapshot *entities.Snapshot,
	status ports.WorkspaceStatus,
	haveStatus bool,
) (string, error) {
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

// Workspace is one git read, held for the length of a single workflow.
//
// It exists because "checkpoint the session" and "rewrite context.md" both need
// the workspace state, and running them back to back used to read git twice:
// Checkpoint.Create built a snapshot (one `git status`), then WriteContextMD went
// through Get, which read git again, rebuilt the same snapshot and re-ran the
// staleness check the checkpoint had just satisfied. Every Stop hook and every
// session.record paid for both. It is passed explicitly and never cached: two
// calls in the same workflow see one state, and the next workflow reads git again.
type Workspace struct {
	Status ports.WorkspaceStatus
	// Have is false when git is unavailable or the project path cannot be
	// resolved; callers then skip sync, staleness and nudges rather than guessing.
	Have bool
}

// ReadWorkspace performs the one git read a workflow is allowed to share.
func (s *ContextService) ReadWorkspace(ctx context.Context, sessionID string) Workspace {
	status, have := s.gitStatus(ctx, sessionID)
	return Workspace{Status: status, Have: have}
}

// SnapshotFrom builds the checkpoint snapshot from an already-read workspace.
func (s *ContextService) SnapshotFrom(ctx context.Context, sessionID string, ws Workspace) (*entities.Snapshot, error) {
	return s.checkpoints.BuildSnapshotWith(ctx, sessionID, ws.Status, ws.Have)
}

// WriteContextMDFrom renders context.md from a snapshot the caller already built.
//
// Unlike WriteContextMD it neither syncs file changes nor auto-checkpoints: its
// callers have just written a checkpoint from this very snapshot, and doing either
// again would re-read git and store a second checkpoint for the same state.
func (s *ContextService) WriteContextMDFrom(
	ctx context.Context,
	root, sessionID string,
	snapshot *entities.Snapshot,
	ws Workspace,
) (string, error) {
	text, err := s.render(ctx, sessionID, ContextDepthSummary, snapshot, ws.Status, ws.Have)
	if err != nil {
		return "", err
	}
	return s.writeContextFile(root, text)
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

// WriteContextMD writes context.md into the .agent dir (human-readable). It goes
// through Get, so it syncs file changes and may auto-checkpoint; a caller that
// already holds a snapshot wants WriteContextMDFrom instead.
func (s *ContextService) WriteContextMD(ctx context.Context, root, sessionID string) (string, error) {
	text, err := s.Get(ctx, sessionID, ContextDepthSummary)
	if err != nil {
		return "", err
	}
	return s.writeContextFile(root, text)
}

func (s *ContextService) writeContextFile(root, text string) (string, error) {
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
