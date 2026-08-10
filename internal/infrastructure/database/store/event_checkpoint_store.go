package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/agent-session/agent-session/internal/domain/entities"
	domainerr "github.com/agent-session/agent-session/internal/domain/errors"
	"github.com/agent-session/agent-session/internal/domain/repositories"
)

type eventStore struct {
	db *gorm.DB
}

func (s *eventStore) Append(ctx context.Context, event *entities.SessionEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func (s *eventStore) ListBySession(ctx context.Context, sessionID string, limit int) ([]*entities.SessionEvent, error) {
	var events []*entities.SessionEvent
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events, nil
}

type checkpointStore struct {
	db *gorm.DB
}

func (s *checkpointStore) Create(ctx context.Context, checkpoint *entities.Checkpoint) error {
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(checkpoint).Error; err != nil {
		return fmt.Errorf("create checkpoint: %w", err)
	}
	return nil
}

func (s *checkpointStore) GetLatest(ctx context.Context, sessionID string) (*entities.Checkpoint, error) {
	var cp entities.Checkpoint
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		First(&cp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrCheckpointNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest checkpoint: %w", err)
	}
	return &cp, nil
}

func (s *checkpointStore) ListBySession(ctx context.Context, sessionID string, limit int) ([]*entities.Checkpoint, error) {
	var checkpoints []*entities.Checkpoint
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&checkpoints).Error; err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	return checkpoints, nil
}

var (
	_ repositories.EventRepository      = (*eventStore)(nil)
	_ repositories.CheckpointRepository = (*checkpointStore)(nil)
)
