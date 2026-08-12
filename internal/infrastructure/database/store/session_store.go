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

type sessionStore struct {
	db *gorm.DB
}

func (s *sessionStore) Create(ctx context.Context, session *entities.Session) error {
	now := entities.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// StartExclusive completes every active session of the project and inserts the
// new one inside a single transaction, returning the ids it superseded.
//
// Starting a session used to read the active session and then write, so two
// processes starting at once both saw no active session and both created one.
// Five concurrent starts left five active sessions, after which GetActive picked
// one arbitrarily and the project's state was split across them.
func (s *sessionStore) StartExclusive(ctx context.Context, session *entities.Session) ([]string, error) {
	now := entities.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}

	var superseded []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entities.Session{}).
			Where("project_id = ? AND status = ?", session.ProjectID, entities.SessionStatusActive).
			Pluck("id", &superseded).Error; err != nil {
			return fmt.Errorf("find active sessions: %w", err)
		}
		if len(superseded) > 0 {
			if err := tx.Model(&entities.Session{}).
				Where("id IN ?", superseded).
				Updates(map[string]interface{}{
					"status":     entities.SessionStatusCompleted,
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("complete active sessions: %w", err)
			}
		}
		if err := tx.Create(session).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return superseded, nil
}

func (s *sessionStore) GetByID(ctx context.Context, id string) (*entities.Session, error) {
	var sess entities.Session
	err := s.db.WithContext(ctx).First(&sess, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session by id: %w", err)
	}
	return &sess, nil
}

func (s *sessionStore) GetActive(ctx context.Context, projectID string) (*entities.Session, error) {
	var sess entities.Session
	err := s.db.WithContext(ctx).
		Where("project_id = ? AND status = ?", projectID, entities.SessionStatusActive).
		Order("updated_at DESC").
		First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrNoActiveSession
	}
	if err != nil {
		return nil, fmt.Errorf("get active session: %w", err)
	}
	return &sess, nil
}

func (s *sessionStore) GetLatest(ctx context.Context, projectID string) (*entities.Session, error) {
	var sess entities.Session
	err := s.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("updated_at DESC").
		First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest session: %w", err)
	}
	return &sess, nil
}

func (s *sessionStore) Update(ctx context.Context, session *entities.Session) error {
	session.UpdatedAt = entities.Now()
	if err := s.db.WithContext(ctx).Save(session).Error; err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	return nil
}

func (s *sessionStore) ListByProject(ctx context.Context, projectID string, limit, offset int) ([]*entities.Session, error) {
	var sessions []*entities.Session
	if err := s.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("updated_at DESC").
		Limit(limit).Offset(offset).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

var _ repositories.SessionRepository = (*sessionStore)(nil)
