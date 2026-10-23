package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// errInjected is the failure these tests plant partway through a use case.
var errInjected = errors.New("injected repository failure")

// brokenApp wires a second app over the fixture's database with appends of failOn
// rejected. The failure seam itself lives in failinject_test.go, which can plant a
// failure at any repository write; this is the common case of one event type.
func brokenApp(t *testing.T, f *fixture, failOn string) *app.App {
	t.Helper()
	return brokenAppWith(t, f, failEventType(failOn))
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

// TestStartRollsBackWhenAgentSessionInsertFails plants the failure on the write
// after StartExclusive instead of on the event, because the two halves fail
// differently: StartExclusive marks the project's active sessions completed and
// inserts the new one, and a failure in the agent-session insert that followed
// used to leave the project with the old session completed, the new one present,
// and no agent attached to either.
func TestStartRollsBackWhenAgentSessionInsertFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	first := initRes.Session

	broken := brokenAppWith(t, f, failAt(repoAgentSessions, opCreate))
	if _, err := broken.Session.Start(ctx, first.ProjectID, "second", "codex"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	sessions, err := f.app.Store.Sessions().ListByProject(ctx, first.ProjectID, 50, 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != first.ID {
		t.Fatalf("a rolled-back start left %d sessions, want only %s", len(sessions), first.ID)
	}
	if sessions[0].Status != entities.SessionStatusActive {
		t.Errorf("the original session is %q; StartExclusive's completion was not rolled back", sessions[0].Status)
	}
	// finalizeSuperseded closed this one before the insert failed, so it is the
	// assertion that the rollback reached the whole transaction and not just the
	// write that failed.
	if open := openAgentSessions(t, f, first.ID); open != 1 {
		t.Errorf("the original session has %d open agent sessions, want 1", open)
	}
}

// TestCompleteRollsBackWhenEventFails verifies session.complete is one change: a
// session marked completed whose agent sessions were closed with nothing in the
// log to say so reads, to the next agent, as a session that ended for no reason.
func TestCompleteRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	broken := brokenApp(t, f, entities.EventSessionCompleted)
	if err := broken.Session.Complete(ctx, session.ID); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	current, err := f.app.Store.Sessions().GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if current.Status != entities.SessionStatusActive {
		t.Errorf("session status is %q after a failed complete, want active", current.Status)
	}
	if open := openAgentSessions(t, f, session.ID); open != 1 {
		t.Errorf("a rolled-back complete left %d open agent sessions, want 1", open)
	}
}

// TestResumeRollsBackWhenAgentSessionFails plants the failure on the agent-session
// swap rather than the event: reviving the session and attaching the agent are
// separate writes, and a session left active whose agent session never opened
// reads as "an agent is working on this" with nothing behind it.
func TestResumeRollsBackWhenAgentSessionFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session
	if err := f.app.Session.Complete(ctx, session.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	broken := brokenAppWith(t, f, failAt(repoAgentSessions, opResume))
	if _, err := broken.Session.Resume(ctx, session.ProjectID, "codex"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	current, err := f.app.Store.Sessions().GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if current.Status != entities.SessionStatusCompleted {
		t.Errorf("session status is %q after a failed resume, want completed", current.Status)
	}
	if current.LastAgent == "codex" {
		t.Error("a rolled-back resume left the session attributed to the agent that never resumed it")
	}
	if types := eventTypes(t, f, session.ID); containsType(types, entities.EventSessionResumed) {
		t.Errorf("session.resumed was recorded for a resume that failed: %v", types)
	}
}

// TestTaskUpdateRollsBackWhenEventFails verifies the status a task carries and the
// event announcing the move commit together: the event's payload is documented to
// name the status the task actually reached.
func TestTaskUpdateRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session
	task, err := f.app.Task.Create(ctx, session.ID, "wire the port", "claude")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	broken := brokenApp(t, f, entities.EventTaskUpdated)
	if _, err := broken.Task.Update(ctx, task.ID, "", entities.TaskStatusCompleted, "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	current, err := f.app.Store.Tasks().GetByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if current.Status != entities.TaskStatusInProgress {
		t.Errorf("task status is %q after a failed update, want it unchanged at %q",
			current.Status, entities.TaskStatusInProgress)
	}
}

// TestBlockerCreateRollsBackWhenEventFails verifies a blocker never exists without
// the event that puts it on the timeline — a blocker the log never mentions is
// invisible to anything reading the session as a history.
func TestBlockerCreateRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	broken := brokenApp(t, f, entities.EventBlockerCreated)
	if _, err := broken.Decision.CreateBlocker(ctx, session.ID, "waiting on the vendor", "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	blockers, err := f.app.Store.Blockers().ListBySession(ctx, session.ID)
	if err != nil {
		t.Fatalf("list blockers: %v", err)
	}
	if len(blockers) != 0 {
		t.Errorf("a rolled-back blocker create left %d rows behind", len(blockers))
	}
}

// TestBlockerResolveRollsBackWhenUpdateFails covers the resolve path from the
// other side: the row update fails, so the assertion is that no blocker.resolved
// event survives. An event announcing a resolution the blockers table never
// applied is worse than no event, because the timeline is what an agent reads.
func TestBlockerResolveRollsBackWhenUpdateFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session
	blocker, err := f.app.Decision.CreateBlocker(ctx, session.ID, "waiting on the vendor", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	broken := brokenAppWith(t, f, failAt(repoBlockers, opResolve))
	if err := broken.Decision.ResolveBlocker(ctx, blocker.ID); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	if types := eventTypes(t, f, session.ID); containsType(types, entities.EventBlockerResolved) {
		t.Errorf("blocker.resolved was recorded although the row was never updated: %v", types)
	}
	open, err := f.app.Store.Blockers().ListOpen(ctx, session.ID)
	if err != nil {
		t.Fatalf("list open blockers: %v", err)
	}
	if len(open) != 1 {
		t.Errorf("the blocker is no longer open after a failed resolve: %d open", len(open))
	}
}

// TestCheckpointRollsBackWhenRowFails is the other direction of the checkpoint
// guarantee. The existing test fails the event and checks no checkpoint survives;
// this one fails the checkpoint insert and checks no checkpoint.created survives,
// because an event carrying a checkpoint_id that resolves to nothing breaks the
// one promise the payload contract makes about it.
func TestCheckpointRollsBackWhenRowFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session
	before := countType(eventTypes(t, f, session.ID), entities.EventCheckpointCreated)

	broken := brokenAppWith(t, f, failAt(repoCheckpoints, opCreate))
	if _, err := broken.Checkpoint.Create(ctx, session.ID, "manual", "next", "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	if after := countType(eventTypes(t, f, session.ID), entities.EventCheckpointCreated); after != before {
		t.Errorf("a refused checkpoint left %d announcing events behind", after-before)
	}
}

// TestOffloadedAppendRollsBackWhenArtifactFails covers the first half of the
// offload: a payload over LargePayloadThreshold is stored as an artifact and the
// event carries only the reference, so a failed artifact write must not leave an
// event whose artifact_id resolves to nothing.
func TestOffloadedAppendRollsBackWhenArtifactFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session
	before := countType(eventTypes(t, f, session.ID), entities.EventCommandExecuted)

	broken := brokenAppWith(t, f, failAt(repoArtifacts, opCreate))
	if err := broken.Artifact.AppendEvent(ctx, session.ID, "claude",
		entities.EventCommandExecuted, oversizePayload()); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	if after := countType(eventTypes(t, f, session.ID), entities.EventCommandExecuted); after != before {
		t.Errorf("a refused artifact left %d events pointing at nothing", after-before)
	}
}

// TestOffloadedAppendRollsBackWhenEventFails covers the other half: the artifact
// is written first, so a failed event append must not leave the payload stored with
// nothing referencing it — an artifact no event names can never be read again and
// is counted by nothing that prunes.
func TestOffloadedAppendRollsBackWhenEventFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	// The store exposes no artifact listing, so the id the service generated is
	// how this test can ask whether the row survived the rollback.
	var stored []string
	inject := watch(failAt(repoEvents, opAppend), repoArtifacts, opCreate, func(arg any) {
		if artifact, ok := arg.(*entities.Artifact); ok {
			stored = append(stored, artifact.ID)
		}
	})

	broken := brokenAppWith(t, f, inject)
	if err := broken.Artifact.AppendEvent(ctx, session.ID, "claude",
		entities.EventCommandExecuted, oversizePayload()); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	if len(stored) != 1 {
		t.Fatalf("the oversize payload was not offloaded at all (%d artifact writes), so this test proves nothing", len(stored))
	}
	if _, err := f.app.Artifact.Get(ctx, session.ID, stored[0]); err == nil {
		t.Errorf("artifact %s survived the rolled-back append with no event referencing it", stored[0])
	}
}

// TestPromoteRollbackLeavesNoIndexedKnowledge covers the knowledge write that
// touches two tables. Each Create inserts the row and its FTS index entry, and the
// promote sweep runs several of them in one transaction: a rollback that reached
// the rows but not the index would leave memory.search returning hits that resolve
// to nothing, which is not visible from the knowledge table alone.
func TestPromoteRollbackLeavesNoIndexedKnowledge(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	session := initRes.Session

	for i := 0; i < 3; i++ {
		if _, err := f.app.Decision.Create(ctx, session.ID,
			"promote candidate about rotating refresh tokens", "atomicity", "claude"); err != nil {
			t.Fatalf("create decision: %v", err)
		}
	}

	// the second row of the sweep, so the first is already written when it fails
	broken := brokenAppWith(t, f, failAtNth(repoKnowledge, opCreate, 2))
	if _, err := broken.Memory.Promote(ctx, session.ID, "claude"); !errors.Is(err, errInjected) {
		t.Fatalf("expected the injected failure to surface, got %v", err)
	}

	rows, err := f.app.Store.Knowledge().ListBySession(ctx, session.ID, "", 100)
	if err != nil {
		t.Fatalf("list knowledge: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("a rolled-back promote left %d knowledge rows behind", len(rows))
	}
	hits, err := f.app.Memory.Search(ctx, "rotating refresh tokens", 100)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("the FTS index holds %d entries the knowledge table does not have", len(hits))
	}
}

// oversizePayload returns a valid JSON payload over LargePayloadThreshold, which
// is what sends AppendEvent down the artifact-offload path.
func oversizePayload() string {
	payload, err := json.Marshal(map[string]string{
		"stdout": strings.Repeat("x", app.LargePayloadThreshold+1),
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

func countType(values []string, want string) int {
	n := 0
	for _, v := range values {
		if v == want {
			n++
		}
	}
	return n
}
