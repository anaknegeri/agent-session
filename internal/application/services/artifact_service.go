package services

import (
	"context"
	"fmt"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

// LargePayloadThreshold is the size above which event payloads are stored as
// artifacts and referenced instead of inline (PRD §14.3).
const LargePayloadThreshold = 8 * 1024

// MaxPayloadSize caps a single event payload. Offloading to an artifact bounds
// what the events table carries but not what the process holds: an agent piping
// a whole build log or a binary through event.append had the entire string in
// memory and then again in SQLite. Anything genuinely that large belongs in a
// file the diff can point at.
const MaxPayloadSize = 1 << 20

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
//
// The offloaded artifact and the event referencing it commit together, so a
// failed append cannot leave a stored payload nothing points at.
func (s *ArtifactService) AppendEvent(ctx context.Context, sessionID, agent, eventType, payload string) error {
	if !entities.IsCanonicalEventType(eventType) {
		return domainerr.ErrInvalidEventType
	}
	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("event payload is %d bytes, over the %d byte limit", len(payload), MaxPayloadSize)
	}
	if len(payload) <= LargePayloadThreshold {
		return s.appendEvent(ctx, s.store, sessionID, agent, eventType, payload)
	}
	return s.store.Tx(ctx, func(st ports.Store) error {
		artifactID, err := s.withStore(st).Store(ctx, sessionID, artifactKind(eventType), "", payload)
		if err != nil {
			return err
		}
		return s.appendEvent(ctx, st, sessionID, agent, eventType, fmt.Sprintf(`{"artifact_id":"%s"}`, artifactID))
	})
}

func (s *ArtifactService) appendEvent(ctx context.Context, st ports.Store, sessionID, agent, eventType, payload string) error {
	return st.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     agent,
		Type:      eventType,
		Payload:   payload,
	})
}

// withStore returns a copy bound to st, so the artifact is written inside a
// transaction the caller has already opened.
func (s *ArtifactService) withStore(st ports.Store) *ArtifactService {
	scoped := *s
	scoped.store = st
	return &scoped
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
