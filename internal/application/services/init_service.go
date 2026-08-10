package services

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type InitResult struct {
	Project *entities.Project
	Session *entities.Session
	IsGit   bool // whether the project is inside a git repository
}

type InitService struct {
	store  ports.Store
	git    ports.GitService
	logger *slog.Logger
}

func NewInitService(store ports.Store, git ports.GitService, logger *slog.Logger) *InitService {
	return &InitService{store: store, git: git, logger: logger}
}

// Init sets up the project, creates the initial session and records session.started
// (UC-01). Not a git repository is allowed — session works in local mode.
func (s *InitService) Init(ctx context.Context, dir, agent string) (*InitResult, error) {
	isGit, err := s.git.Detect(ctx, dir)
	if err != nil {
		return nil, domainerr.ErrNotGitRepo
	}

	projectName := filepath.Base(dir)
	workspace := ports.WorkspaceStatus{}
	if isGit {
		workspace, err = s.git.Status(ctx, dir)
		if err != nil {
			s.logger.Warn("git status failed, continuing local-only", "error", err)
		}
		if workspace.Repository != "" {
			projectName = workspace.Repository
		}
	}

	project, err := s.store.Projects().GetByPath(ctx, dir)
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	if isNotFound(err) {
		project = &entities.Project{
			ID:   ids.New("proj"),
			Name: projectName,
			Path: dir,
		}
		if err := s.store.Projects().Create(ctx, project); err != nil {
			return nil, err
		}
	}

	active, err := s.store.Sessions().GetActive(ctx, project.ID)
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	if active != nil {
		return &InitResult{Project: project, Session: active, IsGit: isGit}, nil
	}

	session := &entities.Session{
		ID:        ids.New("sess"),
		ProjectID: project.ID,
		Title:     "",
		Status:    entities.SessionStatusActive,
		Branch:    workspace.Branch,
		Commit:    workspace.Commit,
		Dirty:     workspace.Dirty,
		LastAgent: agent,
	}
	if err := s.store.Sessions().Create(ctx, session); err != nil {
		return nil, err
	}

	if err := s.openAgentSession(ctx, session.ID, agent); err != nil {
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

	s.logger.Info("session started",
		"session_id", session.ID,
		"project", project.Name,
		"agent", agent,
	)

	return &InitResult{Project: project, Session: session, IsGit: isGit}, nil
}

func (s *InitService) openAgentSession(ctx context.Context, sessionID, agent string) error {
	agentSession := &entities.AgentSession{
		ID:        ids.New("asess"),
		SessionID: sessionID,
		Agent:     agent,
	}
	if err := s.store.AgentSessions().Create(ctx, agentSession); err != nil {
		return err
	}
	return nil
}

func isNotFound(err error) bool {
	return err == domainerr.ErrProjectNotFound ||
		err == domainerr.ErrSessionNotFound ||
		err == domainerr.ErrNoActiveSession
}
