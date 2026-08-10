package services

import (
	"context"
	"log/slog"

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
	task := &entities.Task{
		ID:        ids.New("task"),
		SessionID: sessionID,
		Title:     title,
		Status:    entities.TaskStatusInProgress,
	}
	if err := s.store.Tasks().Create(ctx, task); err != nil {
		return nil, err
	}
	if err := s.setCurrentTask(ctx, sessionID, task.ID); err != nil {
		return nil, err
	}
	if session, err := s.store.Sessions().GetByID(ctx, sessionID); err == nil {
		if session.Title == "" {
			session.Title = title
			if err := s.store.Sessions().Update(ctx, session); err != nil {
				return nil, err
			}
		}
	}
	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     agent,
		Type:      entities.EventTaskCreated,
		Payload:   `{"task_id":"` + task.ID + `"}`,
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
	if err := s.store.Tasks().Update(ctx, task); err != nil {
		return nil, err
	}
	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: task.SessionID,
		Agent:     agent,
		Type:      entities.EventTaskUpdated,
		Payload:   `{"task_id":"` + task.ID + `","status":"` + task.Status + `"}`,
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

func (s *TaskService) setCurrentTask(ctx context.Context, sessionID, taskID string) error {
	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	session.CurrentTaskID = taskID
	return s.store.Sessions().Update(ctx, session)
}
