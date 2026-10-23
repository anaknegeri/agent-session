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

// ListBySession lists the decisions of a session, as an empty slice rather than
// nil when there are none: decision.list and session://decisions marshal this
// straight out over MCP, and gorm leaves a nil slice for "no rows" — `null` where
// the contract promises a list.
func (s *decisionStore) ListBySession(ctx context.Context, sessionID string) ([]*entities.Decision, error) {
	decisions := []*entities.Decision{}
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

func (s *blockerStore) GetByID(ctx context.Context, id string) (*entities.Blocker, error) {
	var blocker entities.Blocker
	if err := s.db.WithContext(ctx).First(&blocker, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerr.ErrBlockerNotFound
		}
		return nil, fmt.Errorf("get blocker: %w", err)
	}
	return &blocker, nil
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
	// Empty, not nil: blocker.list hands this to an agent over MCP, and a session
	// with nothing blocked is the common case.
	blockers := []*entities.Blocker{}
	if err := q.Order("created_at ASC").Find(&blockers).Error; err != nil {
		return nil, fmt.Errorf("list blockers: %w", err)
	}
	return blockers, nil
}

func (s *blockerStore) Resolve(ctx context.Context, id string, resolvedAt interface{}) error {
	result := s.db.WithContext(ctx).
		Model(&entities.Blocker{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": entities.BlockerStatusResolved, "resolved_at": resolvedAt})
	if result.Error != nil {
		return fmt.Errorf("resolve blocker: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domainerr.ErrBlockerNotFound
	}
	return nil
}

var (
	_ repositories.DecisionRepository = (*decisionStore)(nil)
	_ repositories.BlockerRepository  = (*blockerStore)(nil)
)
