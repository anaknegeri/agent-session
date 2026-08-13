package services

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/pkg/ids"
	"github.com/anaknegeri/agent-session/pkg/safetext"
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
	// Stored as an identifier for the same reason SessionService.Start does it: the
	// name is rendered as the session layer's own assertion, not as agent prose.
	agent = safetext.Identifier(agent)

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

	// Everything from here reads the store before writing to it, so it runs as one
	// transaction: two agents running init at once would otherwise both see no
	// active session and both create one, and a failure part-way would leave a
	// session with no agent session and no session.started to explain it.
	var result *InitResult
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		project, err := st.Projects().GetByPath(ctx, dir)
		if err != nil && !isNotFound(err) {
			return err
		}
		if isNotFound(err) {
			project = &entities.Project{
				ID:   ids.New("proj"),
				Name: projectName,
				Path: dir,
			}
			if err := st.Projects().Create(ctx, project); err != nil {
				return err
			}
		}

		active, err := st.Sessions().GetActive(ctx, project.ID)
		if err != nil && !isNotFound(err) {
			return err
		}
		if active != nil {
			result = &InitResult{Project: project, Session: active, IsGit: isGit}
			return nil
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
		if err := st.Sessions().Create(ctx, session); err != nil {
			return err
		}

		if err := st.AgentSessions().Create(ctx, &entities.AgentSession{
			ID:        ids.New("asess"),
			SessionID: session.ID,
			Agent:     agent,
		}); err != nil {
			return err
		}

		if err := st.Events().Append(ctx, &entities.SessionEvent{
			ID:        ids.New("evt"),
			SessionID: session.ID,
			Agent:     agent,
			Type:      entities.EventSessionStarted,
		}); err != nil {
			return err
		}

		s.logger.Info("session started",
			"session_id", session.ID,
			"project", project.Name,
			"agent", agent,
		)
		result = &InitResult{Project: project, Session: session, IsGit: isGit}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func isNotFound(err error) bool {
	return err == domainerr.ErrProjectNotFound ||
		err == domainerr.ErrSessionNotFound ||
		err == domainerr.ErrNoActiveSession
}
