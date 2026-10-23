package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// AppendEvent records a canonical event. It is the only append path a service or
// an entrypoint may use: the closed type namespace, the payload size cap and the
// artifact offload are applied here and nowhere else, so a caller that reaches for
// EventRepository.Append directly skips all three.
//
// Payloads larger than LargePayloadThreshold are stored as an artifact and
// referenced by ID. The artifact and the event referencing it commit together, so
// a failed append cannot leave a stored payload nothing points at.
func (s *ArtifactService) AppendEvent(ctx context.Context, sessionID, agent, eventType, payload string) error {
	payload, err := eventPayload(eventType, payload)
	if err != nil {
		return err
	}
	if len(payload) <= LargePayloadThreshold {
		return s.appendEvent(ctx, s.store, sessionID, agent, eventType, payload)
	}
	return s.store.Tx(ctx, func(st ports.Store) error {
		return s.offloadAndAppend(ctx, st, sessionID, agent, eventType, payload)
	})
}

// AppendEventTx is AppendEvent for a caller that has already opened a
// transaction, so the event commits with the state change it describes instead of
// separately from it. It deliberately opens no transaction of its own: GORM turns
// a nested one into a SAVEPOINT, which buys nothing here because the caller's
// boundary already covers both writes.
func (s *ArtifactService) AppendEventTx(ctx context.Context, st ports.Store, sessionID, agent, eventType, payload string) error {
	payload, err := eventPayload(eventType, payload)
	if err != nil {
		return err
	}
	if len(payload) <= LargePayloadThreshold {
		return s.appendEvent(ctx, st, sessionID, agent, eventType, payload)
	}
	return s.offloadAndAppend(ctx, st, sessionID, agent, eventType, payload)
}

// Get returns an offloaded payload. The session scopes the lookup: an artifact_id
// read out of one session's event log resolves to nothing in another.
func (s *ArtifactService) Get(ctx context.Context, sessionID, id string) (*entities.Artifact, error) {
	return s.store.Artifacts().GetByID(ctx, id, sessionID)
}

// eventPayload validates a caller-supplied payload and returns what to store.
//
// An empty payload becomes `{}`: event-v1.md promises `payload` is JSON text and
// `{}` when there is nothing to say, and a row holding an empty string makes
// every reader either parse that as JSON or special-case it.
func eventPayload(eventType, payload string) (string, error) {
	if !entities.IsCanonicalEventType(eventType) {
		return "", domainerr.ErrInvalidEventType
	}
	if len(payload) > MaxPayloadSize {
		return "", fmt.Errorf("event payload is %d bytes, over the %d byte limit", len(payload), MaxPayloadSize)
	}
	if strings.TrimSpace(payload) == "" {
		return entities.EmptyPayload, nil
	}
	return payload, nil
}

func (s *ArtifactService) offloadAndAppend(ctx context.Context, st ports.Store, sessionID, agent, eventType, payload string) error {
	artifactID, err := s.withStore(st).Store(ctx, sessionID, artifactKind(eventType), "", payload)
	if err != nil {
		return err
	}
	ref, err := json.Marshal(map[string]string{"artifact_id": artifactID})
	if err != nil {
		return fmt.Errorf("marshal artifact reference: %w", err)
	}
	return s.appendEvent(ctx, st, sessionID, agent, eventType, string(ref))
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
