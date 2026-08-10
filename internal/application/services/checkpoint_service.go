package services

import (
	"context"
	"encoding/json"
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

	return &entities.Snapshot{
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
	}, nil
}

func (s *CheckpointService) testsState(ctx context.Context, sessionID string) entities.TestsState {
	events, err := s.store.Events().ListBySession(ctx, sessionID, 100)
	if err != nil {
		return entities.TestsState{}
	}
	state := entities.TestsState{Status: "unknown"}
	failed := 0
	for _, e := range events {
		switch e.Type {
		case entities.EventTestFailed:
			failed++
			state.Status = "failed"
		case entities.EventTestPassed:
			if state.Status == "unknown" {
				state.Status = "passed"
			}
		}
	}
	state.Failures = failed
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
