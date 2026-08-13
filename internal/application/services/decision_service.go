package services

import (
	"context"
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type DecisionService struct {
	store  ports.Store
	logger *slog.Logger
}

func NewDecisionService(store ports.Store, logger *slog.Logger) *DecisionService {
	return &DecisionService{store: store, logger: logger}
}

func (s *DecisionService) Create(ctx context.Context, sessionID, decision, reason, agent string) (*entities.Decision, error) {
	d := &entities.Decision{
		ID:        ids.New("decision"),
		SessionID: sessionID,
		Decision:  decision,
		Reason:    reason,
		Agent:     agent,
	}
	// The decision and the event pointing at it commit together: an agent reading
	// the timeline follows decision_id and must find the row, and a decision no
	// event announces never shows up in the log.
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		if err := st.Decisions().Create(ctx, d); err != nil {
			return err
		}
		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: sessionID,
			Agent:     agent,
			Type:      entities.EventDecisionCreated,
			Payload:   `{"decision_id":"` + d.ID + `"}`,
		})
	}); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DecisionService) List(ctx context.Context, sessionID string) ([]*entities.Decision, error) {
	return s.store.Decisions().ListBySession(ctx, sessionID)
}

func (s *DecisionService) CreateBlocker(ctx context.Context, sessionID, description, agent string) (*entities.Blocker, error) {
	b := &entities.Blocker{
		ID:          ids.New("blocker"),
		SessionID:   sessionID,
		Description: description,
		Status:      entities.BlockerStatusOpen,
		Agent:       agent,
	}
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		if err := st.Blockers().Create(ctx, b); err != nil {
			return err
		}
		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: sessionID,
			Agent:     agent,
			Type:      entities.EventBlockerCreated,
			Payload:   `{"blocker_id":"` + b.ID + `"}`,
		})
	}); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *DecisionService) ListBlockers(ctx context.Context, sessionID string, openOnly bool) ([]*entities.Blocker, error) {
	if openOnly {
		return s.store.Blockers().ListOpen(ctx, sessionID)
	}
	return s.store.Blockers().ListBySession(ctx, sessionID)
}

func (s *DecisionService) ResolveBlocker(ctx context.Context, id string) error {
	return s.store.Blockers().Resolve(ctx, id, entities.Now())
}
