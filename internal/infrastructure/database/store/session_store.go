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
