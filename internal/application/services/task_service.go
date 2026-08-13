package services

import (
	"context"
	"log/slog"
	"strings"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type TaskService struct {
	store  ports.Store
	logger *slog.Logger
}

func NewTaskService(store ports.Store, logger *slog.Logger) *TaskService {
	return &TaskService{store: store, logger: logger}
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
		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: sessionID,
			Agent:     agent,
			Type:      entities.EventTaskCreated,
			Payload:   `{"task_id":"` + task.ID + `"}`,
		})
	}); err != nil {
		return nil, err
	}
	s.logger.Info("task created", "task_id", task.ID, "title", title)
	return task, nil
}

// Update updates task fields and emits task.updated.
func (s *TaskService) Update(ctx context.Context, taskID, title, status, agent string) (*entities.Task, error) {
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
	// actually stored.
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		if err := st.Tasks().Update(ctx, task); err != nil {
			return err
		}
		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: task.SessionID,
			Agent:     agent,
			Type:      entities.EventTaskUpdated,
			Payload:   `{"task_id":"` + task.ID + `","status":"` + task.Status + `"}`,
		})
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
