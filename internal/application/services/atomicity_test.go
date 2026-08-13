package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
	"github.com/anaknegeri/agent-session/internal/wire"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// errInjected is the failure these tests plant partway through a use case.
var errInjected = errors.New("injected append failure")

// failingEvents fails every Append of one event type and passes the rest through,
// which cuts a multi-write use case in half at a known point.
type failingEvents struct {
	repositories.EventRepository
	failOn string
}

func (f *failingEvents) Append(ctx context.Context, event *entities.SessionEvent) error {
	if event.Type == f.failOn {
		return errInjected
	}
	return f.EventRepository.Append(ctx, event)
}

// brokenStore is a ports.Store whose event repository fails on one event type.
// Tx re-wraps the transaction-scoped store it is handed, otherwise the injected
// failure would disappear the moment a service opened a transaction — which is
// exactly the code path under test.
type brokenStore struct {
	ports.Store
	failOn string
}

func (s *brokenStore) Events() repositories.EventRepository {
	return &failingEvents{EventRepository: s.Store.Events(), failOn: s.failOn}
}

func (s *brokenStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.Store.Tx(ctx, func(inner ports.Store) error {
		return fn(&brokenStore{Store: inner, failOn: s.failOn})
	})
}

// brokenApp wires a second app over the fixture's database, with appends of
// failOn rejected. Both apps share one store, so assertions can be made through
// the healthy one.
func brokenApp(t *testing.T, f *fixture, failOn string) *app.App {
	t.Helper()
	broken, err := wire.InitializeApp(
		&brokenStore{Store: f.app.Store, failOn: failOn},
		logger.New("error"),
		ports.DefaultContextBudget(),
		config.RetentionConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return broken
}

func eventTypes(t *testing.T, f *fixture, sessionID string) []string {
	t.Helper()
	events, err := f.app.Store.Events().ListBySession(context.Background(), sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	types := make([]string, 0, len(events))
	for _, e := range events {
		types = append(types, e.Type)
	}
	return types
}

func containsType(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// TestStartRollsBackWhenSupersedeBookkeepingFails is the regression test for the
// split state that StartExclusive used to leave behind: it committed the new
// session and the completion of the old one, and only then closed the old agent
// session and appended session.completed. A failure there left the old session
// completed with its agent session still open and no event explaining it.
func TestStartRollsBackWhenSupersedeBookkeepingFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	first := initRes.Session

	broken := brokenApp(t, f, entities.EventSessionCompleted)
	if _, err := broken.Session.Start(ctx, first.ProjectID, "second", "codex"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	sessions, err := f.app.Store.Sessions().ListByProject(ctx, first.ProjectID, 50, 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("a rolled-back start left %d sessions, want 1", len(sessions))
	}
	if sessions[0].ID != first.ID {
		t.Fatalf("the surviving session is %s, want the original %s", sessions[0].ID, first.ID)
	}
	if sessions[0].Status != entities.SessionStatusActive {
		t.Errorf("the original session is %q; StartExclusive's completion was not rolled back", sessions[0].Status)
	}
	if open := openAgentSessions(t, f, first.ID); open != 1 {
		t.Errorf("the original session has %d open agent sessions, want 1", open)
	}
	if types := eventTypes(t, f, first.ID); containsType(types, entities.EventSessionCompleted) {
		t.Errorf("session.completed was recorded for a session that is still active: %v", types)
	}
}

// TestInitRollsBackWhenEventFails verifies a failed init leaves no session
// behind: one that exists without session.started, or with no agent session, is
// worse than none, because resume finds it and reports an agent already working.
func TestInitRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	broken := brokenApp(t, f, entities.EventSessionStarted)
	if _, err := broken.Init.Init(ctx, f.dir, "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	project, err := f.app.Store.Projects().GetByPath(ctx, f.dir)
	if err != nil {
		// The project row is created in the same transaction, so a rolled-back init
		// leaves nothing to look up — which is the assertion.
		return
	}
	sessions, err := f.app.Store.Sessions().ListByProject(ctx, project.ID, 50, 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("a rolled-back init left %d sessions behind", len(sessions))
	}
}

// TestTaskCreateRollsBackWhenEventFails verifies the session never points at a
// current task that was not written.
func TestTaskCreateRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	broken := brokenApp(t, f, entities.EventTaskCreated)
	if _, err := broken.Task.Create(ctx, session.ID, "wire the port", "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	current, err := f.app.Store.Sessions().GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if current.CurrentTaskID != "" {
		t.Errorf("current_task_id is %q after a failed task create, want empty", current.CurrentTaskID)
	}
	if current.Title != "" {
		t.Errorf("session title is %q after a failed task create, want empty", current.Title)
	}
	tasks, err := f.app.Store.Tasks().ListBySession(ctx, session.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("a rolled-back task create left %d tasks behind", len(tasks))
	}
}

// TestDecisionRollsBackWhenEventFails verifies a decision is never stored
// without the event that puts it on the timeline.
func TestDecisionRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	broken := brokenApp(t, f, entities.EventDecisionCreated)
	if _, err := broken.Decision.Create(ctx, session.ID, "use Store.Tx", "atomicity", "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	decisions, err := f.app.Store.Decisions().ListBySession(ctx, session.ID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("a rolled-back decision left %d rows behind", len(decisions))
	}
}

// TestCheckpointRollsBackWhenEventFails verifies a checkpoint is never stored
// without the checkpoint.created event that makes it visible to the timeline.
func TestCheckpointRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	before, err := f.app.Store.Checkpoints().ListBySession(ctx, session.ID, 50)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}

	broken := brokenApp(t, f, entities.EventCheckpointCreated)
	if _, err := broken.Checkpoint.Create(ctx, session.ID, "manual", "next", "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	after, err := f.app.Store.Checkpoints().ListBySession(ctx, session.ID, 50)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("a rolled-back checkpoint changed the count %d -> %d", len(before), len(after))
	}
}

// TestHandoffRollsBackWhenEventFails verifies a failed handoff does not move the
// session to an agent that never received the context.
func TestHandoffRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	before, err := f.app.Store.Checkpoints().ListBySession(ctx, session.ID, 50)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}

	broken := brokenApp(t, f, entities.EventHandoffCreated)
	if _, err := broken.Handoff.Handoff(ctx, session.ID, "codex"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	current, err := f.app.Store.Sessions().GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if current.LastAgent != "claude" {
		t.Errorf("last_agent is %q after a failed handoff, want claude", current.LastAgent)
	}
	after, err := f.app.Store.Checkpoints().ListBySession(ctx, session.ID, 50)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("a rolled-back handoff changed the checkpoint count %d -> %d", len(before), len(after))
	}
}
