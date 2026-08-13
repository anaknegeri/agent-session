package services

import (
	"context"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// EventService is the read side of the event log. Appending goes through
// ArtifactService.AppendEvent: that is the only place the closed type namespace
// (docs/spec/event-v1.md) and the payload size and offload rules are applied, and
// a second append entry point here would let a caller skip all three.
type EventService struct {
	store ports.Store
}

func NewEventService(store ports.Store) *EventService {
	return &EventService{store: store}
}

func (s *EventService) List(ctx context.Context, sessionID string, limit int) ([]*entities.SessionEvent, error) {
	return s.store.Events().ListBySession(ctx, sessionID, limit)
}
