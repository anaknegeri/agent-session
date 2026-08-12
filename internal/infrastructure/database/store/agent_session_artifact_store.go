package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
	"github.com/anaknegeri/agent-session/pkg/ids"
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

// ListOpen returns agent sessions that have not been ended yet.
func (s *agentSessionStore) ListOpen(ctx context.Context, sessionID string) ([]*entities.AgentSession, error) {
	var rows []*entities.AgentSession
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND ended_at IS NULL", sessionID).
		Order("started_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list open agent sessions: %w", err)
	}
	return rows, nil
}

// Resume closes all open agent sessions and creates a new one inside a single
// transaction, so concurrent callers never end up with more than one active
// agent session per session.
func (s *agentSessionStore) Resume(ctx context.Context, sessionID, agent string) (*entities.AgentSession, error) {
	var created *entities.AgentSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entities.AgentSession{}).
			Where("session_id = ? AND ended_at IS NULL", sessionID).
			Updates(map[string]interface{}{"ended_at": entities.Now()}).Error; err != nil {
			return fmt.Errorf("close open agent sessions: %w", err)
		}

		as := &entities.AgentSession{
			ID:        ids.New("asess"),
			SessionID: sessionID,
			Agent:     agent,
		}
		if err := tx.Create(as).Error; err != nil {
			return fmt.Errorf("create agent session: %w", err)
		}
		created = as
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
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
