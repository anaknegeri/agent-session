package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type agentSessionStore struct {
	db *gorm.DB
}

func (s *agentSessionStore) Create(ctx context.Context, agentSession *entities.AgentSession) error {
	if agentSession.StartedAt.IsZero() {
		agentSession.StartedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(agentSession).Error; err != nil {
		return fmt.Errorf("create agent session: %w", err)
	}
	return nil
}

func (s *agentSessionStore) Close(ctx context.Context, id string, endedAt interface{}, checkpointID string) error {
	updates := map[string]interface{}{"ended_at": endedAt}
	if checkpointID != "" {
		updates["checkpoint_id"] = checkpointID
	}
	if err := s.db.WithContext(ctx).
		Model(&entities.AgentSession{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("close agent session: %w", err)
	}
	return nil
}

func (s *agentSessionStore) GetLatest(ctx context.Context, sessionID string) (*entities.AgentSession, error) {
	var as entities.AgentSession
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("started_at DESC").
		First(&as).Error; err != nil {
		return nil, fmt.Errorf("get latest agent session: %w", err)
	}
	return &as, nil
}

type artifactStore struct {
	db *gorm.DB
}

func (s *artifactStore) Create(ctx context.Context, artifact *entities.Artifact) error {
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(artifact).Error; err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}
	return nil
}

var (
	_ repositories.AgentSessionRepository = (*agentSessionStore)(nil)
	_ repositories.ArtifactRepository     = (*artifactStore)(nil)
)
