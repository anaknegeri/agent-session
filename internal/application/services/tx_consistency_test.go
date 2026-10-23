package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
	"github.com/anaknegeri/agent-session/internal/infrastructure/database"
	"github.com/anaknegeri/agent-session/internal/wire"
	"github.com/anaknegeri/agent-session/pkg/ids"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// errReadFailed is the failure these tests plant in a repository read or write.
var errReadFailed = errors.New("injected repository failure")

// failingDecisionsStore fails the decision listing an export depends on, which is
// one of the six reads that used to discard its error.
type failingDecisionsStore struct {
	ports.Store
}

func (s *failingDecisionsStore) Decisions() repositories.DecisionRepository {
	return failingDecisions{DecisionRepository: s.Store.Decisions()}
}

type failingDecisions struct {
	repositories.DecisionRepository
}

func (failingDecisions) ListBySession(context.Context, string) ([]*entities.Decision, error) {
	return nil, errReadFailed
}

// failingKnowledgeStore fails knowledge creates after failAfter of them, cutting a
// promote sweep in half at a known point. Tx re-wraps the transaction-scoped store
// it is handed, or the injected failure would vanish the moment the service opened
// a transaction — which is the path under test.
type failingKnowledgeStore struct {
	ports.Store
	failAfter int
	created   int
}

func (s *failingKnowledgeStore) Knowledge() repositories.KnowledgeRepository {
	return &failingKnowledge{KnowledgeRepository: s.Store.Knowledge(), owner: s}
}

func (s *failingKnowledgeStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.Store.Tx(ctx, func(scoped ports.Store) error {
		return fn(&failingKnowledgeStore{Store: scoped, failAfter: s.failAfter, created: s.created})
	})
}

type failingKnowledge struct {
	repositories.KnowledgeRepository
	owner *failingKnowledgeStore
}

func (k *failingKnowledge) Create(ctx context.Context, entry *entities.Knowledge) error {
	if k.owner.created >= k.owner.failAfter {
		return errReadFailed
	}
	k.owner.created++
	return k.KnowledgeRepository.Create(ctx, entry)
}

// pruneSpyStore records the scope every checkpoint prune runs in, so a prune held
// inside the caller's write transaction can be told from one that runs after the
// commit.
type pruneSpyStore struct {
	ports.Store
	scoped bool
	scopes *[]bool
}

func (s *pruneSpyStore) Checkpoints() repositories.CheckpointRepository {
	return &pruneSpy{CheckpointRepository: s.Store.Checkpoints(), scoped: s.scoped, scopes: s.scopes}
}

func (s *pruneSpyStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.Store.Tx(ctx, func(inner ports.Store) error {
		return fn(&pruneSpyStore{Store: inner, scoped: true, scopes: s.scopes})
	})
}

type pruneSpy struct {
	repositories.CheckpointRepository
	scoped bool
	scopes *[]bool
}

func (p *pruneSpy) PruneKind(ctx context.Context, sessionID, kind string, keep int) (int, error) {
	*p.scopes = append(*p.scopes, p.scoped)
	return p.CheckpointRepository.PruneKind(ctx, sessionID, kind, keep)
}

// failingEventsStore fails every append of one event type, which cuts a state
// change in half at a known point. Tx re-wraps the scoped store so the failure
// survives the transaction under test.
type failingEventsStore struct {
	ports.Store
	failOn string
}

func (s *failingEventsStore) Events() repositories.EventRepository {
	return &failingAppend{EventRepository: s.Store.Events(), failOn: s.failOn}
}

func (s *failingEventsStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.Store.Tx(ctx, func(inner ports.Store) error {
		return fn(&failingEventsStore{Store: inner, failOn: s.failOn})
	})
}

type failingAppend struct {
	repositories.EventRepository
	failOn string
}

func (f *failingAppend) Append(ctx context.Context, event *entities.SessionEvent) error {
	if event.Type == f.failOn {
		return errReadFailed
	}
	return f.EventRepository.Append(ctx, event)
}

// dirtyStatus is a workspace status carrying n changed paths, so the sync service
// has something to record without git being involved.
func dirtyStatus(n int) ports.WorkspaceStatus {
	changes := make([]ports.FileChange, 0, n)
	for i := range n {
		changes = append(changes, ports.FileChange{Path: fmt.Sprintf("internal/generated/file%d.go", i), Status: "M"})
	}
	return ports.WorkspaceStatus{Branch: "main", Dirty: true, Changes: changes}
}

// TestSyncFileChangesOffloadsOversizePayload is the regression test for the one
// append that bypassed the canonical path. SyncFileChanges built its
// payload from every unrecorded git path and wrote it straight to the event
// repository, so the 1 MiB cap and the artifact offload — the two rules that exist
// because that payload is unbounded — did not apply to it. It runs on every
// context.get.
func TestSyncFileChangesOffloadsOversizePayload(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	// enough changed paths that the payload crosses the offload threshold
	status := dirtyStatus(app.LargePayloadThreshold/24 + 40)
	n, err := fx.app.Sync.SyncFileChanges(ctx, sessionID, "claude", status)
	if err != nil {
		t.Fatalf("sync file changes: %v", err)
	}
	if n == 0 {
		t.Fatal("no files were recorded")
	}

	events, err := fx.app.Event.List(ctx, sessionID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var payload string
	for _, ev := range events {
		if ev.Type == entities.EventFileChanged {
			payload = ev.Payload
			break
		}
	}
	if payload == "" {
		t.Fatal("no file.changed event recorded")
	}
	if len(payload) > app.LargePayloadThreshold {
		t.Fatalf("a %d byte file.changed payload was stored inline; the offload rule did not apply", len(payload))
	}
	var ref struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.Unmarshal([]byte(payload), &ref); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if ref.ArtifactID == "" {
		t.Fatalf("an oversize file.changed payload carries no artifact reference: %s", payload)
	}
	artifact, err := fx.app.Artifact.Get(ctx, sessionID, ref.ArtifactID)
	if err != nil {
		t.Fatalf("the offloaded file list is unreachable: %v", err)
	}
	if !strings.Contains(artifact.Content, "file0.go") {
		t.Errorf("the offloaded artifact does not carry the file list: %.80s", artifact.Content)
	}
}

// TestTaskUpdatePayloadSurvivesQuotedStatus covers the payload that was built by
// concatenating a caller-controlled value into JSON: a status carrying a quote
// produced a payload no reader could parse, on an event whose status key the spec
// guarantees. ValidTaskStatus now rejects such a value at the service boundary,
// so the hostile value can no longer reach the marshal — but the marshal is the
// defence in depth, and is exercised here directly rather than through the gate.
func TestTaskUpdatePayloadSurvivesQuotedStatus(t *testing.T) {
	for _, status := range []string{
		`completed","task_id":"forged`,
		`done\`,
		`back\\slash`,
	} {
		payload, err := json.Marshal(map[string]string{"task_id": "task_x", "status": status})
		if err != nil {
			t.Fatalf("marshal status %q: %v", status, err)
		}
		var got struct {
			TaskID string `json:"task_id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatalf("status %q produced a payload no reader can parse (%s): %v", status, payload, err)
		}
		if got.Status != status {
			t.Errorf("status %q round-tripped as %q", status, got.Status)
		}
		if got.TaskID != "task_x" {
			t.Errorf("the hostile status forged task_id %q from %s", got.TaskID, payload)
		}
	}
}

// TestBlockerResolveRecordsEventInOneTransaction covers a state change that used
// to leave no trace: resolving a blocker updated the row with no event and no
// transaction, so the log — documented as the canonical history of state changes —
// still showed the blocker open.
func TestBlockerResolveRecordsEvent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	blocker, err := fx.app.Decision.CreateBlocker(ctx, initRes.Session.ID, "waiting on review", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	if err := fx.app.Decision.ResolveBlocker(ctx, blocker.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}

	events, err := fx.app.Event.List(ctx, initRes.Session.ID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events {
		if ev.Type != entities.EventBlockerResolved {
			continue
		}
		var payload struct {
			BlockerID string `json:"blocker_id"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Fatalf("blocker.resolved payload is not JSON: %v", err)
		}
		if payload.BlockerID != blocker.ID {
			t.Errorf("blocker.resolved names %q, want %q", payload.BlockerID, blocker.ID)
		}
		return
	}
	t.Fatalf("resolving a blocker recorded no %s event", entities.EventBlockerResolved)
}

// TestSessionResumeRecordsEvent covers the other silent state change: reviving a
// completed session left the log claiming it was still completed.
func TestSessionResumeRecordsEvent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := fx.app.Session.Complete(ctx, initRes.Session.ID); err != nil {
		t.Fatalf("complete session: %v", err)
	}
	if _, err := fx.app.Session.Resume(ctx, initRes.Project.ID, "codex"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	events, err := fx.app.Event.List(ctx, initRes.Session.ID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events {
		if ev.Type == entities.EventSessionResumed {
			if ev.Agent != "codex" {
				t.Errorf("session.resumed is attributed to %q, want the resuming agent", ev.Agent)
			}
			return
		}
	}
	t.Fatalf("resuming a session recorded no %s event", entities.EventSessionResumed)
}

// TestImportLeavesOneActiveSession is the regression test for an import that
// created its session with a plain insert: the project's existing active session
// stayed active, so GetActive picked between two of them arbitrarily and the next
// tool call could land in either.
func TestImportLeavesOneActiveSession(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := fx.app.Task.Create(ctx, initRes.Session.ID, "wire the port", "claude"); err != nil {
		t.Fatalf("create task: %v", err)
	}
	data, err := fx.app.Export.ExportJSON(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// imported back into the same project, which already has a live session
	importedID, err := fx.app.Export.Import(ctx, initRes.Project.ID, data, "codex")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	sessions, err := fx.app.Store.Sessions().ListByProject(ctx, initRes.Project.ID, 50, 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	active := make([]string, 0, len(sessions))
	for _, s := range sessions {
		if s.Status == entities.SessionStatusActive {
			active = append(active, s.ID)
		}
	}
	if len(active) != 1 {
		t.Fatalf("an import left %d active sessions in the project (%v); exactly one may be active", len(active), active)
	}
	if active[0] != importedID {
		t.Errorf("the active session is %q, want the imported %q", active[0], importedID)
	}

	// the superseded session is finalized the way Start finalizes one: no agent
	// session left open, and a session.completed explaining it
	open, err := fx.app.Store.AgentSessions().ListOpen(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("list open agent sessions: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("the superseded session still has %d open agent sessions", len(open))
	}
	if !hasEventType(t, fx, initRes.Session.ID, entities.EventSessionCompleted) {
		t.Errorf("the superseded session has no %s event", entities.EventSessionCompleted)
	}
}

// TestExportScopesMemoryToSession is the regression test for the export that
// listed knowledge with no predicate at all: exporting one session shipped every
// knowledge row in the database — every other session, every other project — to
// whoever received the file.
func TestExportScopesMemoryToSession(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	mine, err := fx.app.Memory.Put(ctx, initRes.Session.ID, entities.KnowledgeKindProject, "this session's note", "claude")
	if err != nil {
		t.Fatalf("put memory: %v", err)
	}

	// another session in the same database, holding a secret of its own
	other := &entities.Session{ID: ids.New("sess"), ProjectID: initRes.Project.ID, Status: entities.SessionStatusCompleted}
	if err := fx.app.Store.Sessions().Create(ctx, other); err != nil {
		t.Fatalf("create other session: %v", err)
	}
	if _, err := fx.app.Memory.Put(ctx, other.ID, entities.KnowledgeKindProject, "STAGING_TOKEN=hunter2", "codex"); err != nil {
		t.Fatalf("put other memory: %v", err)
	}

	exp, err := fx.app.Export.Export(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(exp.Memory) != 1 {
		t.Fatalf("the export carries %d memory entries, want only this session's 1", len(exp.Memory))
	}
	if exp.Memory[0].ID != mine.ID {
		t.Errorf("the export carries knowledge %q, want %q", exp.Memory[0].ID, mine.ID)
	}
	data, err := fx.app.Export.ExportJSON(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	if strings.Contains(string(data), "STAGING_TOKEN") {
		t.Error("the export document leaked another session's knowledge")
	}
}

// TestExportReportsFailedRead covers the six reads that discarded their error: a
// failed query dropped a whole collection and the export still reported success,
// so an import of that file recreated a session quietly missing its decisions.
func TestExportReportsFailedRead(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := fx.app.Decision.Create(ctx, initRes.Session.ID, "freeze it", "readers depend on it", "claude"); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	broken := app.NewExportService(&failingDecisionsStore{Store: fx.app.Store}, fx.app.Session, fx.app.Artifact)
	if _, err := broken.Export(ctx, initRes.Session.ID); !errors.Is(err, errReadFailed) {
		t.Fatalf("a failed collection read did not surface, got %v", err)
	}
}

// TestRetentionPrunesAnnouncingEvents covers the dangling reference retention used
// to leave: it deleted checkpoint rows and left the checkpoint.created events
// carrying their ids, so a reader following checkpoint_id — which the payload
// contract guarantees resolves — got "checkpoint not found" instead.
func TestRetentionPrunesAnnouncingEvents(t *testing.T) {
	fx := newFixtureWithRetention(t, 2)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	for i := range 5 {
		if _, err := fx.app.Checkpoint.CreateKind(ctx, sessionID, entities.CheckpointKindAuto, "auto", "", "claude"); err != nil {
			t.Fatalf("checkpoint %d: %v", i, err)
		}
	}

	events, err := fx.app.Event.List(ctx, sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	announced := 0
	for _, ev := range events {
		if ev.Type != entities.EventCheckpointCreated {
			continue
		}
		announced++
		var payload struct {
			CheckpointID string `json:"checkpoint_id"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Fatalf("checkpoint.created payload is not JSON: %v", err)
		}
		if _, err := fx.app.Checkpoint.GetByID(ctx, payload.CheckpointID); err != nil {
			t.Errorf("checkpoint.created points at %s, which does not resolve: %v", payload.CheckpointID, err)
		}
	}
	if announced == 0 {
		t.Fatal("no checkpoint.created events survived; retention removed more than it announced")
	}
	if announced == 5 {
		t.Fatalf("all 5 announcing events survived a keep-2 retention limit; nothing was pruned")
	}
}

// TestPromoteIsAllOrNothing covers the partial promote: N unguarded creates that
// reported the count reached when one failed, so a caller was told how many rows
// were promoted over a database that held none of them.
func TestPromoteIsAllOrNothing(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID
	for _, d := range []string{"use sqlite", "keep it local", "no daemon"} {
		if _, err := fx.app.Decision.Create(ctx, sessionID, d, "because", "claude"); err != nil {
			t.Fatalf("create decision: %v", err)
		}
	}

	broken := app.NewMemoryService(&failingKnowledgeStore{Store: fx.app.Store, failAfter: 2}, logger.New("error"))
	count, err := broken.Promote(ctx, sessionID, "claude")
	if !errors.Is(err, errReadFailed) {
		t.Fatalf("the injected create failure did not surface, got %v", err)
	}
	if count != 0 {
		t.Errorf("a rolled-back promote reported %d promoted entries", count)
	}

	stored, err := fx.app.Memory.ListByKind(ctx, "", 100)
	if err != nil {
		t.Fatalf("list knowledge: %v", err)
	}
	if len(stored) != 0 {
		t.Errorf("a rolled-back promote left %d knowledge rows behind", len(stored))
	}
}

// TestArtifactIsSessionScoped covers the read added for offloaded payloads: an
// artifact_id belongs to one session's log, and holding it must not read the
// payload back through another session.
func TestArtifactIsSessionScoped(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionA := initRes.Session.ID

	big := `{"output":"` + strings.Repeat("x", app.LargePayloadThreshold+1) + `"}`
	if err := fx.app.Artifact.AppendEvent(ctx, sessionA, "claude", entities.EventTestFailed, big); err != nil {
		t.Fatalf("append: %v", err)
	}
	events, err := fx.app.Event.List(ctx, sessionA, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var artifactID string
	for _, ev := range events {
		if ev.Type != entities.EventTestFailed {
			continue
		}
		var ref struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &ref); err != nil {
			t.Fatalf("payload is not JSON: %v", err)
		}
		artifactID = ref.ArtifactID
	}
	if artifactID == "" {
		t.Fatal("the oversize payload was not offloaded")
	}

	artifact, err := fx.app.Artifact.Get(ctx, sessionA, artifactID)
	if err != nil {
		t.Fatalf("the session that wrote the artifact cannot read it: %v", err)
	}
	if artifact.Content != big {
		t.Errorf("the artifact does not carry the payload (%d bytes vs %d)", len(artifact.Content), len(big))
	}

	sessionB, err := fx.app.Session.Start(ctx, initRes.Project.ID, "second", "codex")
	if err != nil {
		t.Fatalf("start second session: %v", err)
	}
	if _, err := fx.app.Artifact.Get(ctx, sessionB.ID, artifactID); !errors.Is(err, domainerr.ErrArtifactNotFound) {
		t.Fatalf("session B read session A's artifact, got err %v", err)
	}
}

func hasEventType(t *testing.T, fx *fixture, sessionID, eventType string) bool {
	t.Helper()
	events, err := fx.app.Event.List(context.Background(), sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

// TestHandoffPrunesOutsideItsTransaction covers where retention runs. Handoff
// created its checkpoint through the transaction-scoped store, and the checkpoint
// service pruned through whatever store it held — so the deletes ran inside the
// handoff transaction, held under its write lock, which is the opposite of what the
// "housekeeping after the commit" comment above them claims.
func TestHandoffPrunesOutsideItsTransaction(t *testing.T) {
	dir := gitProject(t)
	store, err := database.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	var scopes []bool
	spy := &pruneSpyStore{Store: store, scopes: &scopes}
	retention := config.RetentionConfig{MaxManual: 1, MaxAuto: 1, MaxPreCompact: 1, MaxHandoff: 1}
	wired, err := wire.InitializeApp(spy, logger.New("error"), ports.DefaultContextBudget(), retention)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	initRes, err := wired.Init.Init(ctx, dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := wired.Handoff.Handoff(ctx, initRes.Session.ID, "codex"); err != nil {
		t.Fatalf("handoff: %v", err)
	}

	if len(scopes) == 0 {
		t.Fatal("the handoff checkpoint applied no retention at all")
	}
	for i, scoped := range scopes {
		if scoped {
			t.Errorf("prune %d ran inside the caller's transaction; retention must run on the unscoped store after the commit", i)
		}
	}
}

// TestBlockerResolveRollsBackWhenEventFails covers the transaction the resolve
// path did not have: the update and the event announcing it are one change, so a
// blocker never reads as resolved with nothing in the log to say so.
func TestBlockerResolveRollsBackWhenEventFails(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	blocker, err := fx.app.Decision.CreateBlocker(ctx, initRes.Session.ID, "waiting on review", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	broken := app.NewDecisionService(
		&failingEventsStore{Store: fx.app.Store, failOn: entities.EventBlockerResolved},
		app.NewArtifactService(&failingEventsStore{Store: fx.app.Store, failOn: entities.EventBlockerResolved}),
		logger.New("error"),
	)
	if err := broken.ResolveBlocker(ctx, blocker.ID); !errors.Is(err, errReadFailed) {
		t.Fatalf("the injected append failure did not surface, got %v", err)
	}

	open, err := fx.app.Decision.ListBlockers(ctx, initRes.Session.ID, true)
	if err != nil {
		t.Fatalf("list open blockers: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("a rolled-back resolve left %d open blockers, want the unresolved 1", len(open))
	}
}

// gitProject is a git repository for a test that wires its own app instead of
// taking the fixture's.
func gitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "config", "user.email", "t@t.co")
	gitCmd(t, dir, "config", "user.name", "t")
	writeFile(t, dir, "server.go", "package oauth\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-qm", "init")
	return dir
}
