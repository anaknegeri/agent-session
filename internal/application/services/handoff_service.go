package services

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

// supportedHandoffAgents mirrors the adapters documented for `agent-session handoff`.
var supportedHandoffAgents = map[string]bool{
	"claude":   true,
	"codex":    true,
	"opencode": true,
}

type HandoffService struct {
	store       ports.Store
	checkpoints *CheckpointService
	renderer    ports.ContextRenderer
	budget      ports.ContextBudget
	logger      *slog.Logger
}

func NewHandoffService(
	store ports.Store,
	checkpoints *CheckpointService,
	renderer ports.ContextRenderer,
	budget ports.ContextBudget,
	logger *slog.Logger,
) *HandoffService {
	return &HandoffService{store: store, checkpoints: checkpoints, renderer: renderer, budget: budget, logger: logger}
}

// Handoff composes a deterministic handoff context for the target agent (UC-04).
// It snapshots the current state first so the next agent gets the latest
// checkpoint (PRD §23: automatic checkpoint on handoff).
func (s *HandoffService) Handoff(ctx context.Context, sessionID, to string) (string, error) {
	if !supportedHandoffAgents[to] {
		return "", domainerr.ErrAgentNotSupported
	}

	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return "", err
	}

	from := session.LastAgent

	// Built once, before the writes: the handoff checkpoint stores this snapshot
	// and the target agent reads the text rendered from it, so they cannot
	// disagree — and neither git nor the renderer runs while the write lock is held.
	snapshot, err := s.checkpoints.BuildSnapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}

	text, err := s.renderer.RenderHandoff(snapshot, to, s.budget)
	if err != nil {
		return "", err
	}

	handoffID := ids.New("handoff")
	var cp *entities.Checkpoint

	// The checkpoint, the new owning agent and the handoff event are one
	// transition. Half of it applied means the session names an agent that was
	// never handed the context, or a checkpoint no handoff event refers to.
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		var err error
		cp, err = s.checkpoints.withStore(st).CreateFromSnapshot(ctx, sessionID, entities.CheckpointKindHandoff, "handoff", "", from, snapshot)
		if err != nil {
			return err
		}

		session.LastAgent = to
		if err := st.Sessions().Update(ctx, session); err != nil {
			return err
		}

		payload, _ := json.Marshal(map[string]string{
			"handoff_id":    handoffID,
			"from_agent":    from,
			"to_agent":      to,
			"checkpoint_id": cp.ID,
		})
		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: sessionID,
			Agent:     from,
			Type:      entities.EventHandoffCreated,
			Payload:   string(payload),
		})
	}); err != nil {
		return "", err
	}

	s.logger.Info("handoff created",
		"handoff_id", handoffID,
		"session_id", sessionID,
		"from", from,
		"to", to,
		"checkpoint_id", cp.ID,
	)
	return text, nil
}
