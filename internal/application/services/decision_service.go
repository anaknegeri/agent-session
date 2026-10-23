package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type DecisionService struct {
	store    ports.Store
	artifact *ArtifactService
	logger   *slog.Logger
}

func NewDecisionService(store ports.Store, artifact *ArtifactService, logger *slog.Logger) *DecisionService {
	return &DecisionService{store: store, artifact: artifact, logger: logger}
}

func (s *DecisionService) Create(ctx context.Context, sessionID, decision, reason, agent string) (*entities.Decision, error) {
	var d *entities.Decision
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		var err error
		d, err = s.CreateTx(ctx, st, sessionID, decision, reason, agent)
		return err
	}); err != nil {
		return nil, err
	}
	return d, nil
}

// CreateTx records a decision inside a transaction the caller has already opened,
// so a composite write — session.record logs a decision, appends an event and
// checkpoints — commits as one change instead of three the caller cannot undo
// halfway through.
//
// The decision and the event pointing at it commit together: an agent reading the
// timeline follows decision_id and must find the row, and a decision no event
// announces never shows up in the log.
func (s *DecisionService) CreateTx(ctx context.Context, st ports.Store, sessionID, decision, reason, agent string) (*entities.Decision, error) {
	d := &entities.Decision{
		ID:        ids.New("decision"),
		SessionID: sessionID,
		Decision:  decision,
		Reason:    reason,
		Agent:     agent,
	}
	if err := st.Decisions().Create(ctx, d); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]string{"decision_id": d.ID})
	if err != nil {
		return nil, fmt.Errorf("marshal decision payload: %w", err)
	}
	if err := s.artifact.AppendEventTx(ctx, st, sessionID, agent, entities.EventDecisionCreated, string(payload)); err != nil {
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
		payload, err := json.Marshal(map[string]string{"blocker_id": b.ID})
		if err != nil {
			return fmt.Errorf("marshal blocker payload: %w", err)
		}
		return s.artifact.AppendEventTx(ctx, st, sessionID, agent, entities.EventBlockerCreated, string(payload))
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

// ResolveBlocker resolves a blocker and records blocker.resolved.
//
// Resolving used to be a bare update with no event: the log is the canonical
// history of state changes, so a blocker that went from open to resolved left the
// timeline claiming it is still open. The update and the event are one transaction
// for the same reason every other state change is.
//
// The event is attributed to the session's current agent rather than to a caller
// argument, the way session.completed already is — the resolve call carries no
// agent, and inventing one for it would change a signature every entrypoint uses.
func (s *DecisionService) ResolveBlocker(ctx context.Context, id string) error {
	return s.store.Tx(ctx, func(st ports.Store) error {
		blocker, err := st.Blockers().GetByID(ctx, id)
		if err != nil {
			return err
		}
		session, err := st.Sessions().GetByID(ctx, blocker.SessionID)
		if err != nil {
			return err
		}
		if err := st.Blockers().Resolve(ctx, id, entities.Now()); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]string{"blocker_id": id})
		if err != nil {
			return fmt.Errorf("marshal blocker payload: %w", err)
		}
		return s.artifact.AppendEventTx(ctx, st, blocker.SessionID, session.LastAgent, entities.EventBlockerResolved, string(payload))
	})
}
