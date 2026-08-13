package services

import (
	"context"
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
	"github.com/anaknegeri/agent-session/pkg/safetext"
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
	project, err := s.store.Projects().GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	// The agent name is caller-supplied (--agent, the session.resume tool argument,
	// AGENT_SESSION_AGENT) but the rendered context and the handoff document present
	// it as the session layer's own assertion, so it is stored as a single-line
	// identifier and never as prose that could forge a section.
	agent = safetext.Identifier(agent)

	session := &entities.Session{
		ID:        ids.New("sess"),
		ProjectID: project.ID,
		Title:     title,
		Status:    entities.SessionStatusActive,
		Branch:    s.branch(ctx, project.Path),
		LastAgent: agent,
	}
	// Completing the previous session and creating this one must be one step, or
	// two processes starting at once both see no active session and both create
	// one. The bookkeeping that follows belongs in the same step: StartExclusive
	// leaves the superseded sessions marked completed, and a failure while closing
	// their agent sessions or appending their session.completed event would strand
	// a completed session with an open agent session and no event to explain it —
	// plus a new session with no agent session and no session.started.
	err = s.store.Tx(ctx, func(st ports.Store) error {
		superseded, err := st.Sessions().StartExclusive(ctx, session)
		if err != nil {
			return err
		}
		for _, id := range superseded {
			if err := s.finalizeSuperseded(ctx, st, id); err != nil {
				return err
			}
		}

		if err := st.AgentSessions().Create(ctx, &entities.AgentSession{
			ID:        ids.New("asess"),
			SessionID: session.ID,
			Agent:     agent,
		}); err != nil {
			return err
		}

		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: session.ID,
			Agent:     agent,
			Type:      entities.EventSessionStarted,
		})
	})
	if err != nil {
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
	agent = safetext.Identifier(agent)
	// Reviving the session, attaching the agent and swapping the agent session are
	// one state change: a session left active whose agent session never opened
	// reads as "an agent is working on this" with nothing behind it.
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		session.Status = entities.SessionStatusActive
		session.LastAgent = agent
		if err := st.Sessions().Update(ctx, session); err != nil {
			return err
		}
		// closes any open agent session and creates a new one, so exactly one is
		// active even under concurrent processes
		_, err := st.AgentSessions().Resume(ctx, session.ID, agent)
		return err
	}); err != nil {
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
	return s.store.Tx(ctx, func(st ports.Store) error {
		session.Status = entities.SessionStatusCompleted
		if err := st.Sessions().Update(ctx, session); err != nil {
			return err
		}

		// close any agent sessions still open so no session leaks an ended_at=NULL
		if err := s.closeActiveAgentSession(ctx, st, sessionID); err != nil {
			return err
		}

		return st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: session.ID,
			Agent:     session.LastAgent,
			Type:      entities.EventSessionCompleted,
		})
	})
}

// finalizeSuperseded tidies a session that StartExclusive has already marked
// completed: its open agent sessions are closed and session.completed recorded.
// It takes the store to use so the caller can run it inside its transaction.
func (s *SessionService) finalizeSuperseded(ctx context.Context, st ports.Store, sessionID string) error {
	if err := s.closeActiveAgentSession(ctx, st, sessionID); err != nil {
		return err
	}
	session, err := st.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	return st.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     session.LastAgent,
		Type:      entities.EventSessionCompleted,
	})
}

// closeActiveAgentSession ends every agent session for a session that is still
// open (ended_at IS NULL). It is a no-op when none are open.
func (s *SessionService) closeActiveAgentSession(ctx context.Context, st ports.Store, sessionID string) error {
	open, err := st.AgentSessions().ListOpen(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, as := range open {
		if err := st.AgentSessions().Close(ctx, as.ID, entities.Now(), as.CheckpointID); err != nil {
			return err
		}
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
