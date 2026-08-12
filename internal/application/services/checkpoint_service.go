package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type CheckpointService struct {
	store  ports.Store
	git    ports.GitService
	logger *slog.Logger
}

func NewCheckpointService(store ports.Store, git ports.GitService, logger *slog.Logger) *CheckpointService {
	return &CheckpointService{store: store, git: git, logger: logger}
}

// Create builds the canonical snapshot (PRD §15) and stores it.
func (s *CheckpointService) Create(ctx context.Context, sessionID, label, nextAction, agent string) (*entities.Checkpoint, error) {
	snapshot, err := s.BuildSnapshot(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if nextAction != "" {
		snapshot.NextAction = nextAction
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, domainerr.ErrInvalidSnapshot
	}

	cp := &entities.Checkpoint{
		ID:         ids.New("chk"),
		SessionID:  sessionID,
		TaskID:     snapshot.Task.ID,
		Label:      label,
		Snapshot:   string(data),
		NextAction: snapshot.NextAction,
		Agent:      agent,
	}
	if err := s.store.Checkpoints().Create(ctx, cp); err != nil {
		return nil, err
	}

	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     agent,
		Type:      entities.EventCheckpointCreated,
		Payload:   `{"checkpoint_id":"` + cp.ID + `"}`,
	}); err != nil {
		return nil, err
	}

	s.logger.Info("checkpoint created", "checkpoint_id", cp.ID, "session_id", sessionID)
	return cp, nil
}

// BuildSnapshot assembles the canonical state from stores and git.
func (s *CheckpointService) BuildSnapshot(ctx context.Context, sessionID string) (*entities.Snapshot, error) {
	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	project, err := s.store.Projects().GetByID(ctx, session.ProjectID)
	if err != nil {
		return nil, err
	}

	workspace := ports.WorkspaceStatus{Repository: project.Name}
	if s.git != nil {
		if st, gerr := s.git.Status(ctx, project.Path); gerr == nil {
			workspace = st
			if st.Repository == "" {
				workspace.Repository = project.Name
			}
		}
	}

	task, _ := s.store.Tasks().GetByID(ctx, session.CurrentTaskID)
	if task == nil {
		task, _ = s.store.Tasks().GetCurrent(ctx, sessionID)
	}

	allTasks, _ := s.store.Tasks().ListBySession(ctx, sessionID)
	completed := make([]string, 0, len(allTasks))
	pending := make([]string, 0, len(allTasks))
	for _, t := range allTasks {
		if t.Status == entities.TaskStatusCompleted {
			completed = append(completed, t.Title)
		} else {
			pending = append(pending, t.Title)
		}
	}

	decisions, _ := s.store.Decisions().ListBySession(ctx, sessionID)
	blockers, _ := s.store.Blockers().ListOpen(ctx, sessionID)

	modified := make([]string, 0, len(workspace.Changes))
	for _, c := range workspace.Changes {
		modified = append(modified, c.Path)
	}

	snapshot := &entities.Snapshot{
		Session: entities.SessionState{
			ID:     session.ID,
			Title:  session.Title,
			Status: session.Status,
		},
		Workspace: entities.WorkspaceState{
			Repository: workspace.Repository,
			Branch:     workspace.Branch,
			Commit:     workspace.Commit,
			Dirty:      workspace.Dirty,
		},
		Task: entities.TaskState{
			ID:     taskID(task),
			Title:  taskTitle(task),
			Status: taskStatus(task),
		},
		Progress: entities.ProgressState{
			Completed: completed,
			Pending:   pending,
		},
		Decisions:  decisions,
		Files:      entities.FilesState{Modified: modified},
		Tests:      s.testsState(ctx, sessionID),
		Blockers:   blockers,
		NextAction: "",
		LastAgent:  session.LastAgent,
	}

	// surface the most recent checkpoint's next_action so resumed agents know
	// exactly what to continue (UC-05).
	if latest, err := s.store.Checkpoints().GetLatest(ctx, sessionID); err == nil {
		snapshot.NextAction = latest.NextAction
	}

	return snapshot, nil
}

func (s *CheckpointService) testsState(ctx context.Context, sessionID string) entities.TestsState {
	events, err := s.store.Events().ListBySession(ctx, sessionID, 100)
	if err != nil {
		return entities.TestsState{}
	}
	state := entities.TestsState{Status: "unknown"}
	for _, e := range events {
		if state.Status == "unknown" {
			switch e.Type {
			case entities.EventTestFailed:
				state.Status = "failed"
			case entities.EventTestPassed:
				state.Status = "passed"
			}
		}
		if e.Type == entities.EventTestPassed {
			break
		}
		if e.Type == entities.EventTestFailed {
			state.Failures++
		}
	}
	return state
}

func taskID(t *entities.Task) string {
	if t == nil {
		return ""
	}
	return t.ID
}

func taskTitle(t *entities.Task) string {
	if t == nil {
		return ""
	}
	return t.Title
}

func taskStatus(t *entities.Task) string {
	if t == nil {
		return ""
	}
	return t.Status
}

// Latest returns the most recent checkpoint for a session.
func (s *CheckpointService) Latest(ctx context.Context, sessionID string) (*entities.Checkpoint, error) {
	return s.store.Checkpoints().GetLatest(ctx, sessionID)
}

// GetByID returns a checkpoint by ID.
func (s *CheckpointService) GetByID(ctx context.Context, id string) (*entities.Checkpoint, error) {
	return s.store.Checkpoints().GetByID(ctx, id)
}

// ListBySession returns checkpoints for a session, most recent first.
func (s *CheckpointService) ListBySession(ctx context.Context, sessionID string, limit int) ([]*entities.Checkpoint, error) {
	return s.store.Checkpoints().ListBySession(ctx, sessionID, limit)
}

// SnapshotDiff captures what changed between two checkpoints.
type SnapshotDiff struct {
	Before           *entities.Checkpoint `json:"-"`
	After            *entities.Checkpoint `json:"-"`
	TaskTitleChanged bool                 `json:"task_title_changed"`
	TaskStatusFrom   string               `json:"task_status_from"`
	TaskStatusTo     string               `json:"task_status_to"`
	NewDecisions     []*entities.Decision `json:"new_decisions"`
	NewBlockers      []*entities.Blocker  `json:"new_blockers"`
	ResolvedBlockers []*entities.Blocker  `json:"resolved_blockers"`
	NewlyCompleted   []string             `json:"newly_completed"`
	NewlyStarted     []string             `json:"newly_started"`
	NewFiles         []string             `json:"new_files"`
	NextActionFrom   string               `json:"next_action_from"`
	NextActionTo     string               `json:"next_action_to"`
	CommitFrom       string               `json:"commit_from"`
	CommitTo         string               `json:"commit_to"`
}

// HasChanges reports whether the diff contains any changes.
func (d *SnapshotDiff) HasChanges() bool {
	return d.TaskTitleChanged || d.TaskStatusFrom != d.TaskStatusTo ||
		len(d.NewDecisions) > 0 || len(d.NewBlockers) > 0 || len(d.ResolvedBlockers) > 0 ||
		len(d.NewlyCompleted) > 0 || len(d.NewlyStarted) > 0 || len(d.NewFiles) > 0 ||
		d.NextActionFrom != d.NextActionTo || d.CommitFrom != d.CommitTo
}

// Diff compares two checkpoints and returns what changed.
func (s *CheckpointService) Diff(ctx context.Context, beforeID, afterID string) (*SnapshotDiff, error) {
	before, err := s.store.Checkpoints().GetByID(ctx, beforeID)
	if err != nil {
		return nil, fmt.Errorf("get before checkpoint: %w", err)
	}
	after, err := s.store.Checkpoints().GetByID(ctx, afterID)
	if err != nil {
		return nil, fmt.Errorf("get after checkpoint: %w", err)
	}

	beforeSnap, err := s.ParseSnapshot(before)
	if err != nil {
		return nil, err
	}
	afterSnap, err := s.ParseSnapshot(after)
	if err != nil {
		return nil, err
	}

	diff := &SnapshotDiff{Before: before, After: after}

	diff.TaskTitleChanged = beforeSnap.Task.Title != afterSnap.Task.Title
	diff.TaskStatusFrom = beforeSnap.Task.Status
	diff.TaskStatusTo = afterSnap.Task.Status

	beforeDecisions := idSet(len(beforeSnap.Decisions))
	for _, d := range beforeSnap.Decisions {
		beforeDecisions[d.ID] = true
	}
	for _, d := range afterSnap.Decisions {
		if !beforeDecisions[d.ID] {
			diff.NewDecisions = append(diff.NewDecisions, d)
		}
	}

	beforeBlockers := idSet(len(beforeSnap.Blockers))
	for _, b := range beforeSnap.Blockers {
		beforeBlockers[b.ID] = true
	}
	afterBlockersResolved := map[string]*entities.Blocker{}
	for _, b := range afterSnap.Blockers {
		if b.Status == entities.BlockerStatusResolved {
			afterBlockersResolved[b.ID] = b
		}
	}
	for _, b := range beforeSnap.Blockers {
		if beforeBlockers[b.ID] && afterBlockersResolved[b.ID] != nil {
			diff.ResolvedBlockers = append(diff.ResolvedBlockers, b)
		}
	}
	for _, b := range afterSnap.Blockers {
		if !beforeBlockers[b.ID] && b.Status == entities.BlockerStatusOpen {
			diff.NewBlockers = append(diff.NewBlockers, b)
		}
	}

	diff.NewlyCompleted = sliceDiff(afterSnap.Progress.Completed, beforeSnap.Progress.Completed)
	diff.NewlyStarted = sliceDiff(afterSnap.Progress.Pending, beforeSnap.Progress.Pending)

	diff.NewFiles = sliceDiff(afterSnap.Files.Modified, beforeSnap.Files.Modified)

	diff.NextActionFrom = beforeSnap.NextAction
	diff.NextActionTo = afterSnap.NextAction

	diff.CommitFrom = beforeSnap.Workspace.Commit
	diff.CommitTo = afterSnap.Workspace.Commit

	return diff, nil
}

func idSet(n int) map[string]bool {
	if n == 0 {
		return map[string]bool{}
	}
	return make(map[string]bool, n)
}

func sliceDiff(after, before []string) []string {
	bset := make(map[string]bool, len(before))
	for _, s := range before {
		bset[s] = true
	}
	var result []string
	for _, s := range after {
		if !bset[s] {
			result = append(result, s)
		}
	}
	return result
}

// Restore returns the latest checkpoint snapshot for a session.
func (s *CheckpointService) Restore(ctx context.Context, sessionID string) (*entities.Snapshot, error) {
	cp, err := s.store.Checkpoints().GetLatest(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.ParseSnapshot(cp)
}

// ParseSnapshot unmarshals a checkpoint snapshot.
func (s *CheckpointService) ParseSnapshot(cp *entities.Checkpoint) (*entities.Snapshot, error) {
	var snap entities.Snapshot
	if err := json.Unmarshal([]byte(cp.Snapshot), &snap); err != nil {
		return nil, domainerr.ErrInvalidSnapshot
	}
	return &snap, nil
}
