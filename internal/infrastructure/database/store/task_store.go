package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type taskStore struct {
	db *gorm.DB
}

func (s *taskStore) Create(ctx context.Context, task *entities.Task) error {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (s *taskStore) GetByID(ctx context.Context, id string) (*entities.Task, error) {
	var task entities.Task
	err := s.db.WithContext(ctx).First(&task, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get task by id: %w", err)
	}
	return &task, nil
}

func (s *taskStore) GetCurrent(ctx context.Context, sessionID string) (*entities.Task, error) {
	var task entities.Task
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND status IN ?", sessionID, []string{
			entities.TaskStatusInProgress,
			entities.TaskStatusBlocked,
		}).
		Order("updated_at DESC").
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get current task: %w", err)
	}
	return &task, nil
}

func (s *taskStore) Update(ctx context.Context, task *entities.Task) error {
	task.UpdatedAt = entities.Now()
	if err := s.db.WithContext(ctx).Save(task).Error; err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (s *taskStore) ListBySession(ctx context.Context, sessionID string) ([]*entities.Task, error) {
	var tasks []*entities.Task
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

var _ repositories.TaskRepository = (*taskStore)(nil)
