package services_test

import (
	"context"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
	"github.com/anaknegeri/agent-session/internal/wire"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// This is the failure seam for the transaction tests. There is one of it because
// the previous harness could only reject EventRepository.Append selected by event
// type, so every guarantee that does not hinge on the event write — an artifact
// stored with no event pointing at it, a session created with no agent session, a
// checkpoint with no announcing event, a half-promoted knowledge sweep — had no
// way to be tested at all.
//
// A test plants an injector; the injector is consulted before every wrapped
// repository write with the entity that write was called with, so it can fail one
// operation, one event type, or the nth row of a sweep.

// repository names an injector can select. They are the ports.Store accessors, so
// a failure reads as the repository it happens in.
const (
	repoEvents        = "events"
	repoArtifacts     = "artifacts"
	repoCheckpoints   = "checkpoints"
	repoAgentSessions = "agent_sessions"
	repoSessions      = "sessions"
	repoTasks         = "tasks"
	repoDecisions     = "decisions"
	repoBlockers      = "blockers"
	repoKnowledge     = "knowledge"
)

// operations an injector can select.
const (
	opAppend         = "append"
	opCreate         = "create"
	opUpdate         = "update"
	opClose          = "close"
	opResume         = "resume"
	opResolve        = "resolve"
	opStartExclusive = "start_exclusive"
)

// failPoint identifies one repository operation.
type failPoint struct {
	repo string
	op   string
}

func (p failPoint) String() string { return p.repo + "." + p.op }

// injector decides whether the write about to run fails. arg is the entity or the
// id the operation was called with — an *entities.SessionEvent for events.append,
// an *entities.Artifact for artifacts.create — so a hook can select one event type
// or one row of a multi-row sweep. Returning nil lets the write through.
type injector func(p failPoint, arg any) error

// failingStore is a ports.Store that consults an injector before each write.
//
// Tx re-wraps the transaction-scoped store it is handed, otherwise the injected
// failure would disappear the moment a service opened a transaction — which is
// exactly the code path these tests exist for.
type failingStore struct {
	ports.Store
	fail injector
}

func (s *failingStore) check(repo, op string, arg any) error {
	if s.fail == nil {
		return nil
	}
	return s.fail(failPoint{repo: repo, op: op}, arg)
}

func (s *failingStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.Store.Tx(ctx, func(inner ports.Store) error {
		return fn(&failingStore{Store: inner, fail: s.fail})
	})
}

func (s *failingStore) Events() repositories.EventRepository {
	return failEvents{EventRepository: s.Store.Events(), owner: s}
}

func (s *failingStore) Artifacts() repositories.ArtifactRepository {
	return failArtifacts{ArtifactRepository: s.Store.Artifacts(), owner: s}
}

func (s *failingStore) Checkpoints() repositories.CheckpointRepository {
	return failCheckpoints{CheckpointRepository: s.Store.Checkpoints(), owner: s}
}

func (s *failingStore) AgentSessions() repositories.AgentSessionRepository {
	return failAgentSessions{AgentSessionRepository: s.Store.AgentSessions(), owner: s}
}

func (s *failingStore) Sessions() repositories.SessionRepository {
	return failSessions{SessionRepository: s.Store.Sessions(), owner: s}
}

func (s *failingStore) Tasks() repositories.TaskRepository {
	return failTasks{TaskRepository: s.Store.Tasks(), owner: s}
}

func (s *failingStore) Decisions() repositories.DecisionRepository {
	return failDecisions{DecisionRepository: s.Store.Decisions(), owner: s}
}

func (s *failingStore) Blockers() repositories.BlockerRepository {
	return failBlockers{BlockerRepository: s.Store.Blockers(), owner: s}
}

func (s *failingStore) Knowledge() repositories.KnowledgeRepository {
	return failKnowledge{KnowledgeRepository: s.Store.Knowledge(), owner: s}
}

type failEvents struct {
	repositories.EventRepository
	owner *failingStore
}

func (r failEvents) Append(ctx context.Context, event *entities.SessionEvent) error {
	if err := r.owner.check(repoEvents, opAppend, event); err != nil {
		return err
	}
	return r.EventRepository.Append(ctx, event)
}

type failArtifacts struct {
	repositories.ArtifactRepository
	owner *failingStore
}

func (r failArtifacts) Create(ctx context.Context, artifact *entities.Artifact) error {
	if err := r.owner.check(repoArtifacts, opCreate, artifact); err != nil {
		return err
	}
	return r.ArtifactRepository.Create(ctx, artifact)
}

type failCheckpoints struct {
	repositories.CheckpointRepository
	owner *failingStore
}

func (r failCheckpoints) Create(ctx context.Context, cp *entities.Checkpoint) error {
	if err := r.owner.check(repoCheckpoints, opCreate, cp); err != nil {
		return err
	}
	return r.CheckpointRepository.Create(ctx, cp)
}

type failAgentSessions struct {
	repositories.AgentSessionRepository
	owner *failingStore
}

func (r failAgentSessions) Create(ctx context.Context, as *entities.AgentSession) error {
	if err := r.owner.check(repoAgentSessions, opCreate, as); err != nil {
		return err
	}
	return r.AgentSessionRepository.Create(ctx, as)
}

func (r failAgentSessions) Close(ctx context.Context, id string, endedAt interface{}, checkpointID string) error {
	if err := r.owner.check(repoAgentSessions, opClose, id); err != nil {
		return err
	}
	return r.AgentSessionRepository.Close(ctx, id, endedAt, checkpointID)
}

func (r failAgentSessions) Resume(ctx context.Context, sessionID, agent string) (*entities.AgentSession, error) {
	if err := r.owner.check(repoAgentSessions, opResume, sessionID); err != nil {
		return nil, err
	}
	return r.AgentSessionRepository.Resume(ctx, sessionID, agent)
}

type failSessions struct {
	repositories.SessionRepository
	owner *failingStore
}

func (r failSessions) Create(ctx context.Context, session *entities.Session) error {
	if err := r.owner.check(repoSessions, opCreate, session); err != nil {
		return err
	}
	return r.SessionRepository.Create(ctx, session)
}

func (r failSessions) Update(ctx context.Context, session *entities.Session) error {
	if err := r.owner.check(repoSessions, opUpdate, session); err != nil {
		return err
	}
	return r.SessionRepository.Update(ctx, session)
}

func (r failSessions) StartExclusive(ctx context.Context, session *entities.Session) ([]string, error) {
	if err := r.owner.check(repoSessions, opStartExclusive, session); err != nil {
		return nil, err
	}
	return r.SessionRepository.StartExclusive(ctx, session)
}

type failTasks struct {
	repositories.TaskRepository
	owner *failingStore
}

func (r failTasks) Create(ctx context.Context, task *entities.Task) error {
	if err := r.owner.check(repoTasks, opCreate, task); err != nil {
		return err
	}
	return r.TaskRepository.Create(ctx, task)
}

func (r failTasks) Update(ctx context.Context, task *entities.Task) error {
	if err := r.owner.check(repoTasks, opUpdate, task); err != nil {
		return err
	}
	return r.TaskRepository.Update(ctx, task)
}

type failDecisions struct {
	repositories.DecisionRepository
	owner *failingStore
}

func (r failDecisions) Create(ctx context.Context, d *entities.Decision) error {
	if err := r.owner.check(repoDecisions, opCreate, d); err != nil {
		return err
	}
	return r.DecisionRepository.Create(ctx, d)
}

type failBlockers struct {
	repositories.BlockerRepository
	owner *failingStore
}

func (r failBlockers) Create(ctx context.Context, b *entities.Blocker) error {
	if err := r.owner.check(repoBlockers, opCreate, b); err != nil {
		return err
	}
	return r.BlockerRepository.Create(ctx, b)
}

func (r failBlockers) Resolve(ctx context.Context, id string, resolvedAt interface{}) error {
	if err := r.owner.check(repoBlockers, opResolve, id); err != nil {
		return err
	}
	return r.BlockerRepository.Resolve(ctx, id, resolvedAt)
}

type failKnowledge struct {
	repositories.KnowledgeRepository
	owner *failingStore
}

func (r failKnowledge) Create(ctx context.Context, k *entities.Knowledge) error {
	if err := r.owner.check(repoKnowledge, opCreate, k); err != nil {
		return err
	}
	return r.KnowledgeRepository.Create(ctx, k)
}

// --- injectors ---

// failAt fails every call to one repository operation.
func failAt(repo, op string) injector {
	return func(p failPoint, _ any) error {
		if p.repo == repo && p.op == op {
			return errInjected
		}
		return nil
	}
}

// failEventType fails Events().Append for one event type, which cuts a
// multi-write use case in half at a known point.
func failEventType(eventType string) injector {
	return func(p failPoint, arg any) error {
		event, ok := arg.(*entities.SessionEvent)
		if !ok || p.repo != repoEvents || p.op != opAppend {
			return nil
		}
		if event.Type == eventType {
			return errInjected
		}
		return nil
	}
}

// failAtNth fails the nth (1-based) call to one operation and lets the earlier
// ones through, so a sweep is cut mid-way with rows already written.
func failAtNth(repo, op string, nth int) injector {
	seen := 0
	return func(p failPoint, _ any) error {
		if p.repo != repo || p.op != op {
			return nil
		}
		seen++
		if seen == nth {
			return errInjected
		}
		return nil
	}
}

// watch wraps an injector with a callback that sees every matching write before
// it runs. Tests use it to learn the id of a row a rolled-back write would strand:
// the ports.Store exposes no listing for artifacts, so the id the service
// generated is the only way to ask whether it survived.
func watch(inner injector, repo, op string, note func(arg any)) injector {
	return func(p failPoint, arg any) error {
		if p.repo == repo && p.op == op {
			note(arg)
		}
		if inner == nil {
			return nil
		}
		return inner(p, arg)
	}
}

// brokenAppWith wires a second app over the fixture's database with fail planted
// in every repository. Both apps share one store, so assertions can be made
// through the healthy one.
func brokenAppWith(t *testing.T, f *fixture, fail injector) *app.App {
	t.Helper()
	broken, err := wire.InitializeApp(
		&failingStore{Store: f.app.Store, fail: fail},
		logger.New("error"),
		ports.DefaultContextBudget(),
		config.RetentionConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return broken
}
