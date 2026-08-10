package services

import (
	"context"
	"log/slog"

	"github.com/agent-session/agent-session/internal/application/ports"
	"github.com/agent-session/agent-session/internal/domain/entities"
	"github.com/agent-session/agent-session/pkg/ids"
)

type HandoffService struct {
	store       ports.Store
	checkpoints *CheckpointService
	renderer    ports.ContextRenderer
	logger      *slog.Logger
}

func NewHandoffService(
	store ports.Store,
	checkpoints *CheckpointService,
	renderer ports.ContextRenderer,
	logger *slog.Logger,
) *HandoffService {
	return &HandoffService{store: store, checkpoints: checkpoints, renderer: renderer, logger: logger}
}

// Handoff composes a deterministic handoff context for the target agent (UC-04).
// It snapshots the current state first so the next agent gets the latest
// checkpoint (PRD §23: automatic checkpoint on handoff).
func (s *HandoffService) Handoff(ctx context.Context, sessionID, to string) (string, error) {
	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return "", err
	}

	if _, err := s.checkpoints.Create(ctx, sessionID, "handoff", "", session.LastAgent); err != nil {
		return "", err
	}

	snapshot, err := s.checkpoints.BuildSnapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}

	text, err := s.renderer.RenderHandoff(snapshot, to)
	if err != nil {
		return "", err
	}

	session.LastAgent = to
	if err := s.store.Sessions().Update(ctx, session); err != nil {
		return "", err
	}

	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     snapshot.LastAgent,
		Type:      entities.EventHandoffCreated,
		Payload:   `{"to":"` + to + `"}`,
	}); err != nil {
		return "", err
	}

	s.logger.Info("handoff created",
		"session_id", sessionID,
		"from", snapshot.LastAgent,
		"to", to,
	)
	return text, nil
}
