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

func (s *checkpointStore) GetByID(ctx context.Context, id string) (*entities.Checkpoint, error) {
	var cp entities.Checkpoint
	if err := s.db.WithContext(ctx).First(&cp, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerr.ErrCheckpointNotFound
		}
		return nil, fmt.Errorf("get checkpoint: %w", err)
	}
	return &cp, nil
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

// PruneKind deletes the oldest checkpoints of one kind beyond keep and returns
// how many rows were removed.
//
// The session's most recent checkpoint is excluded whatever its kind, because
// resume and restore read that one. For the current caller — which prunes the
// kind it has just written — that exclusion is redundant, since the newest row is
// always among the newest of its own kind; it guards a caller that prunes a kind
// other than the one just created, such as a retention sweep.
func (s *checkpointStore) PruneKind(ctx context.Context, sessionID, kind string, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	res := s.db.WithContext(ctx).Exec(`
		DELETE FROM checkpoints
		WHERE session_id = ? AND kind = ?
		  AND id NOT IN (
		      SELECT id FROM checkpoints
		      WHERE session_id = ? AND kind = ?
		      ORDER BY created_at DESC, id DESC
		      LIMIT ?
		  )
		  AND id <> (
		      SELECT id FROM checkpoints
		      WHERE session_id = ?
		      ORDER BY created_at DESC, id DESC
		      LIMIT 1
		  )`,
		sessionID, kind, sessionID, kind, keep, sessionID)
	if res.Error != nil {
		return 0, fmt.Errorf("prune checkpoints: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}

var (
	_ repositories.EventRepository      = (*eventStore)(nil)
	_ repositories.CheckpointRepository = (*checkpointStore)(nil)
)
