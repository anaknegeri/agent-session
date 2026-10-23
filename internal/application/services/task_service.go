package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type TaskService struct {
	store    ports.Store
	artifact *ArtifactService
	logger   *slog.Logger
}

func NewTaskService(store ports.Store, artifact *ArtifactService, logger *slog.Logger) *TaskService {
	return &TaskService{store: store, artifact: artifact, logger: logger}
}

// Create creates a task and sets it as the current task of the session.
func (s *TaskService) Create(ctx context.Context, sessionID, title, agent string) (*entities.Task, error) {
	if sessionID == "" {
		return nil, domainerr.ErrSessionNotFound
	}
	if strings.TrimSpace(title) == "" {
		return nil, domainerr.ErrTaskTitleRequired
	}
	task := &entities.Task{
		ID:        ids.New("task"),
		SessionID: sessionID,
		Title:     title,
		Status:    entities.TaskStatusInProgress,
	}
	// The task, the session pointing at it as current and the task.created event
	// are one change: a session whose current_task_id names a row that was never
	// written sends the next agent looking for work that does not exist.
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		if err := st.Tasks().Create(ctx, task); err != nil {
			return err
		}
		session, err := st.Sessions().GetByID(ctx, sessionID)
		if err != nil {
			return err
		}
		session.CurrentTaskID = task.ID
		// An untitled session takes the name of its first task, so `session.get`
		// shows what the work is about instead of an empty title.
		if session.Title == "" {
			session.Title = title
		}
		if err := st.Sessions().Update(ctx, session); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]string{"task_id": task.ID})
		if err != nil {
			return fmt.Errorf("marshal task payload: %w", err)
		}
		return s.artifact.AppendEventTx(ctx, st, sessionID, agent, entities.EventTaskCreated, string(payload))
	}); err != nil {
		return nil, err
	}
	s.logger.Info("task created", "task_id", task.ID, "title", title)
	return task, nil
}

// Update updates task fields and emits task.updated.
//
// An unknown status is refused before the store is touched. task status is a
// frozen vocabulary (format-v1.md), and nothing enforced it: a caller sending
// `done` stored `done`, and every reader that classifies a task by status —
// progress in the snapshot, the context document, `session.diff` — either files it
// under the wrong heading or leaves it out. An empty status still means "leave it
// alone", so callers that only update a title are unaffected.
func (s *TaskService) Update(ctx context.Context, taskID, title, status, agent string) (*entities.Task, error) {
	if status != "" && !entities.ValidTaskStatus(status) {
		return nil, fmt.Errorf("%w %q, expected one of: %s",
			domainerr.ErrInvalidTaskStatus, status, strings.Join(entities.TaskStatuses(), ", "))
	}
	task, err := s.store.Tasks().GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if title != "" {
		task.Title = title
	}
	if status != "" {
		task.Status = status
	}
	// The event carries the status it moved to, so it has to be the status that was
	// actually stored — and it is marshalled rather than concatenated: the status is
	// caller-supplied, and a value carrying a quote used to produce a payload no
	// reader could parse.
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		if err := st.Tasks().Update(ctx, task); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]string{"task_id": task.ID, "status": task.Status})
		if err != nil {
			return fmt.Errorf("marshal task payload: %w", err)
		}
		return s.artifact.AppendEventTx(ctx, st, task.SessionID, agent, entities.EventTaskUpdated, string(payload))
	}); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *TaskService) Get(ctx context.Context, id string) (*entities.Task, error) {
	return s.store.Tasks().GetByID(ctx, id)
}

func (s *TaskService) GetCurrent(ctx context.Context, sessionID string) (*entities.Task, error) {
	return s.store.Tasks().GetCurrent(ctx, sessionID)
}

func (s *TaskService) List(ctx context.Context, sessionID string) ([]*entities.Task, error) {
	return s.store.Tasks().ListBySession(ctx, sessionID)
}
