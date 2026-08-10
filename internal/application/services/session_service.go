package services

import (
	"context"
	"log/slog"

	"github.com/agent-session/agent-session/internal/application/ports"
	"github.com/agent-session/agent-session/internal/domain/entities"
	domainerr "github.com/agent-session/agent-session/internal/domain/errors"
	"github.com/agent-session/agent-session/pkg/ids"
)

type SessionService struct {
	store  ports.Store
	git    ports.GitService
	logger *slog.Logger
}

func NewSessionService(store ports.Store, git ports.GitService, logger *slog.Logger) *SessionService {
	return &SessionService{store: store, git: git, logger: logger}
}

func (s *SessionService) Get(ctx context.Context, id string) (*entities.Session, error) {
	if id == "" {
		return nil, domainerr.ErrSessionNotFound
	}
	return s.store.Sessions().GetByID(ctx, id)
}

func (s *SessionService) GetActive(ctx context.Context, projectID string) (*entities.Session, error) {
	return s.store.Sessions().GetActive(ctx, projectID)
}

func (s *SessionService) GetLatest(ctx context.Context, projectID string) (*entities.Session, error) {
	return s.store.Sessions().GetLatest(ctx, projectID)
}

func (s *SessionService) List(ctx context.Context, projectID string, limit, offset int) ([]*entities.Session, error) {
	return s.store.Sessions().ListByProject(ctx, projectID, limit, offset)
}

// Start creates a new session, completing any active session for the project
// and recording session.started.
func (s *SessionService) Start(ctx context.Context, projectID, title, agent string) (*entities.Session, error) {
	if projectID == "" {
		return nil, domainerr.ErrProjectNotFound
	}
	active, err := s.store.Sessions().GetActive(ctx, projectID)
	if err != nil && err != domainerr.ErrNoActiveSession {
		return nil, err
	}
	if active != nil {
		if err := s.complete(ctx, active.ID); err != nil {
			return nil, err
		}
	}

	project, err := s.store.Projects().GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	session := &entities.Session{
		ID:        ids.New("sess"),
		ProjectID: project.ID,
		Title:     title,
		Status:    entities.SessionStatusActive,
		Branch:    s.branch(ctx, project.Path),
		LastAgent: agent,
	}
	if err := s.store.Sessions().Create(ctx, session); err != nil {
		return nil, err
	}

	agentSession := &entities.AgentSession{
		ID:        ids.New("asess"),
		SessionID: session.ID,
		Agent:     agent,
	}
	if err := s.store.AgentSessions().Create(ctx, agentSession); err != nil {
		return nil, err
	}

	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: session.ID,
		Agent:     agent,
		Type:      entities.EventSessionStarted,
	}); err != nil {
		return nil, err
	}

	s.logger.Info("session started", "session_id", session.ID, "title", title, "agent", agent)
	return session, nil
}

// Resume marks the latest session as active again, attaches the current agent
// and records a new agent_session (UC-05).
func (s *SessionService) Resume(ctx context.Context, projectID, agent string) (*entities.Session, error) {
	session, err := s.store.Sessions().GetLatest(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if session.Status != entities.SessionStatusActive {
		session.Status = entities.SessionStatusActive
		if err := s.store.Sessions().Update(ctx, session); err != nil {
			return nil, err
		}
	}

	session.LastAgent = agent
	if err := s.store.Sessions().Update(ctx, session); err != nil {
		return nil, err
	}

	agentSession := &entities.AgentSession{
		ID:        ids.New("asess"),
		SessionID: session.ID,
		Agent:     agent,
	}
	if err := s.store.AgentSessions().Create(ctx, agentSession); err != nil {
		return nil, err
	}

	s.logger.Info("session resumed", "session_id", session.ID, "agent", agent)
	return session, nil
}

func (s *SessionService) Complete(ctx context.Context, sessionID string) error {
	if err := s.complete(ctx, sessionID); err != nil {
		return err
	}
	return nil
}

func (s *SessionService) complete(ctx context.Context, sessionID string) error {
	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	session.Status = entities.SessionStatusCompleted
	if err := s.store.Sessions().Update(ctx, session); err != nil {
		return err
	}

	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: session.ID,
		Agent:     session.LastAgent,
		Type:      entities.EventSessionCompleted,
	}); err != nil {
		return err
	}
	return nil
}

func (s *SessionService) branch(ctx context.Context, dir string) string {
	status, err := s.git.Status(ctx, dir)
	if err != nil {
		return ""
	}
	return status.Branch
}
