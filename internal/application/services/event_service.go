package services

import (
	"context"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type EventService struct {
	store ports.Store
}

func NewEventService(store ports.Store) *EventService {
	return &EventService{store: store}
}

// Append records a canonical event (PRD §14.3). payload is optional raw JSON.
func (s *EventService) Append(ctx context.Context, sessionID, agent, eventType, payload string) error {
	return s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     agent,
		Type:      eventType,
		Payload:   payload,
	})
}

func (s *EventService) List(ctx context.Context, sessionID string, limit int) ([]*entities.SessionEvent, error) {
	return s.store.Events().ListBySession(ctx, sessionID, limit)
}
