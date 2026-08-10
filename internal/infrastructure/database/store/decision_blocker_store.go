package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type decisionStore struct {
	db *gorm.DB
}

func (s *decisionStore) Create(ctx context.Context, decision *entities.Decision) error {
	if decision.CreatedAt.IsZero() {
		decision.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(decision).Error; err != nil {
		return fmt.Errorf("create decision: %w", err)
	}
	return nil
}

func (s *decisionStore) ListBySession(ctx context.Context, sessionID string) ([]*entities.Decision, error) {
	var decisions []*entities.Decision
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&decisions).Error; err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	return decisions, nil
}

type blockerStore struct {
	db *gorm.DB
}

func (s *blockerStore) Create(ctx context.Context, blocker *entities.Blocker) error {
	if blocker.CreatedAt.IsZero() {
		blocker.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(blocker).Error; err != nil {
		return fmt.Errorf("create blocker: %w", err)
	}
	return nil
}

func (s *blockerStore) ListBySession(ctx context.Context, sessionID string) ([]*entities.Blocker, error) {
	return s.list(ctx, sessionID, "")
}

func (s *blockerStore) ListOpen(ctx context.Context, sessionID string) ([]*entities.Blocker, error) {
	return s.list(ctx, sessionID, entities.BlockerStatusOpen)
}

func (s *blockerStore) list(ctx context.Context, sessionID, status string) ([]*entities.Blocker, error) {
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var blockers []*entities.Blocker
	if err := q.Order("created_at ASC").Find(&blockers).Error; err != nil {
		return nil, fmt.Errorf("list blockers: %w", err)
	}
	return blockers, nil
}

func (s *blockerStore) Resolve(ctx context.Context, id string, resolvedAt interface{}) error {
	if err := s.db.WithContext(ctx).
		Model(&entities.Blocker{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": entities.BlockerStatusResolved, "resolved_at": resolvedAt}).
		Error; err != nil {
		return fmt.Errorf("resolve blocker: %w", err)
	}
	return nil
}

var (
	_ repositories.DecisionRepository = (*decisionStore)(nil)
	_ repositories.BlockerRepository  = (*blockerStore)(nil)
)
