package services

import (
	"context"
	"fmt"

	"github.com/agent-session/agent-session/internal/application/ports"
	"github.com/agent-session/agent-session/internal/domain/entities"
	"github.com/agent-session/agent-session/pkg/ids"
)

// LargePayloadThreshold is the size above which event payloads are stored as
// artifacts and referenced instead of inline (PRD §14.3).
const LargePayloadThreshold = 8 * 1024

type ArtifactService struct {
	store ports.Store
}

func NewArtifactService(store ports.Store) *ArtifactService {
	return &ArtifactService{store: store}
}

// Store saves a large payload as an artifact and returns its ID.
func (s *ArtifactService) Store(ctx context.Context, sessionID, kind, path, content string) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("empty artifact content")
	}
	artifact := &entities.Artifact{
		ID:        ids.New("art"),
		SessionID: sessionID,
		Kind:      kind,
		Path:      path,
		Content:   content,
	}
	if err := s.store.Artifacts().Create(ctx, artifact); err != nil {
		return "", err
	}
	return artifact.ID, nil
}

// AppendEvent records a canonical event. Payloads larger than
// LargePayloadThreshold are stored as artifacts and referenced by ID.
func (s *ArtifactService) AppendEvent(ctx context.Context, sessionID, agent, eventType, payload string) error {
	ref := payload
	if len(payload) > LargePayloadThreshold {
		artifactID, err := s.Store(ctx, sessionID, artifactKind(eventType), "", payload)
		if err != nil {
			return err
		}
		ref = fmt.Sprintf(`{"artifact_id":"%s"}`, artifactID)
	}
	return s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     agent,
		Type:      eventType,
		Payload:   ref,
	})
}

func artifactKind(eventType string) string {
	switch eventType {
	case entities.EventTestFailed, entities.EventTestPassed:
		return entities.ArtifactKindTestOutput
	case entities.EventFileChanged:
		return entities.ArtifactKindDiff
	default:
		return entities.ArtifactKindLog
	}
}
