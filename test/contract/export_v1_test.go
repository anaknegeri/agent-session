package contract_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/contract"
)

// exportDocumentV1 is the top-level JSON shape of the export/import document. See
// docs/spec/export-v1.md.
//
// This is a build-boundary artefact exactly like a checkpoint's snapshot: a
// document exported by one build may be imported by an older or newer one, so a
// renamed or removed top-level key is not a refactor, it is a break in a stored
// format.
var exportDocumentV1 = []string{
	"version", "session", "tasks", "decisions", "blockers",
	"events", "checkpoints", "memory", "exported_at",
}

// TestExportDocumentShapeV1 freezes the top-level document shape against
// services.SessionExport's own json tags, so a field renamed or removed there
// fails here rather than surfacing as an import silently missing a collection.
func TestExportDocumentShapeV1(t *testing.T) {
	rt := reflect.TypeOf(services.SessionExport{})
	got := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		name, _ := jsonNameAndOptional(rt.Field(i))
		if name == "" {
			continue
		}
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string(nil), exportDocumentV1...)
	sort.Strings(want)
	assertShape(t, "Export document", "docs/spec/export-v1.md", want, got)
}

// TestExportCollectionsNeverNullV1 holds export-v1.md's "all seven collections
// are always present and [] when empty, never null" clause. session.Start (not
// Init, which triggers an auto-checkpoint through WriteContextMD's stale check)
// gives a session with no tasks, decisions, blockers, checkpoints or memory, and
// exactly one event — genuinely exercising the zero-row path of every list query
// Export makes, not just a zero-value struct marshaled in memory.
func TestExportCollectionsNeverNullV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()

	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.Start(ctx, projectID, "bare session", "claude")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	data, err := app.Export.ExportJSON(ctx, session.ID)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}

	for _, key := range []string{"tasks", "decisions", "blockers", "events", "checkpoints", "memory"} {
		value, ok := raw[key]
		if !ok {
			t.Errorf("an export omits %q entirely\n%s", key, data)
			continue
		}
		list, ok := value.([]any)
		if !ok {
			t.Errorf("%q is %v, want an array (possibly empty), never null\n%s", key, value, data)
			continue
		}
		if key != "events" && len(list) != 0 {
			t.Errorf("%q has %d entries on a session that recorded none: %v", key, len(list), list)
		}
	}
	// events carries the one session.started Start() itself records; it is the
	// collection this test can't force to zero, so it is checked for presence
	// and type above but not emptiness.
	if events, _ := raw["events"].([]any); len(events) == 0 {
		t.Errorf("session.Start must record at least one event, got none\n%s", data)
	}
}

// TestExportCarriesSchemaVersionV1 is what makes the version worth having: an
// export written today has to say which schema it follows.
func TestExportCarriesSchemaVersionV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	data, err := app.Export.ExportJSON(ctx, sessionID)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}
	version, ok := raw["version"]
	if !ok {
		t.Fatalf("the export document has no version field:\n%s", data)
	}
	if got := int(version.(float64)); got != contract.Export {
		t.Errorf("export declares version %d, this build writes v%d", got, contract.Export)
	}
}

// TestImportAcceptsPreVersionedExportV1 covers export files already on disk.
// They predate the version field, and v1 is defined to be their shape, so
// importing one must still succeed.
func TestImportAcceptsPreVersionedExportV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	doc := `{"session":{"title":"pre-version export"}}`
	if _, err := app.Export.Import(ctx, projectID, []byte(doc), "claude"); err != nil {
		t.Fatalf("a version-less export must still import: %v", err)
	}
}

// TestImportRejectsFutureExportVersionV1 is the other half of the promise:
// a document from a schema this build does not understand is refused rather
// than rendered on stale assumptions, the same rule ParseSnapshot applies to a
// checkpoint from a future Checkpoint Schema.
func TestImportRejectsFutureExportVersionV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	doc := fmt.Sprintf(`{"version":%d,"session":{"title":"from the future"}}`, contract.Export+1)
	if _, err := app.Export.Import(ctx, projectID, []byte(doc), "claude"); err == nil {
		t.Fatal("an export from a newer schema version was accepted")
	} else if !strings.Contains(err.Error(), "upgrade agent-session") {
		t.Errorf("the error should tell the user what to do, got: %v", err)
	}
}

// TestImportToleratesUnknownTopLevelKeyV1 freezes export-v1.md's "unknown fields
// in an imported document are ignored, not rejected": a document written by a
// newer build carrying a top-level key this one has no field for must still
// import on this one.
func TestImportToleratesUnknownTopLevelKeyV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	doc := fmt.Sprintf(`{"version":%d,"session":{"title":"forward compatible"},"a_field_this_build_has_never_heard_of":{"x":1}}`, contract.Export)
	if _, err := app.Export.Import(ctx, projectID, []byte(doc), "claude"); err != nil {
		t.Fatalf("an unknown top-level key must not break import: %v", err)
	}
}

// TestImportAppliesDocumentedDropListV1 proves, field by field, the drop list
// export-v1.md commits to: a completed task imports as in_progress, a resolved
// blocker is dropped while an open one survives, a decision's original author is
// replaced by the importing agent, the imported session is always active
// regardless of the exported status, and events/checkpoints/memory are not
// restored at all — the imported session gets nothing from them beyond the one
// fresh session.started Import always writes.
func TestImportAppliesDocumentedDropListV1(t *testing.T) {
	source := newProject(t)
	target := newProject(t)
	ctx := context.Background()

	sourceID := activeSession(t, source)

	task, err := source.Task.Create(ctx, sourceID, "ship the thing", "codex")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := source.Task.Update(ctx, task.ID, "", entities.TaskStatusCompleted, "codex"); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if _, err := source.Decision.Create(ctx, sourceID, "freeze the schema", "readers depend on it", "codex"); err != nil {
		t.Fatalf("create decision: %v", err)
	}
	openBlocker, err := source.Decision.CreateBlocker(ctx, sourceID, "still stuck", "codex")
	if err != nil {
		t.Fatalf("create open blocker: %v", err)
	}
	resolvedBlocker, err := source.Decision.CreateBlocker(ctx, sourceID, "already fixed", "codex")
	if err != nil {
		t.Fatalf("create resolved blocker: %v", err)
	}
	if err := source.Decision.ResolveBlocker(ctx, resolvedBlocker.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}
	if _, err := source.Checkpoint.Create(ctx, sourceID, "manual", "keep going", "codex"); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if _, err := source.Memory.Put(ctx, sourceID, entities.KnowledgeKindSolution, "use a queue", "codex"); err != nil {
		t.Fatalf("put memory: %v", err)
	}
	// the exported session itself is completed, not active
	sourceSession, err := source.Store.Sessions().GetByID(ctx, sourceID)
	if err != nil {
		t.Fatalf("get source session: %v", err)
	}
	sourceSession.Status = entities.SessionStatusCompleted
	if err := source.Store.Sessions().Update(ctx, sourceSession); err != nil {
		t.Fatalf("complete source session: %v", err)
	}

	// sanity: the source really carries what the rest of this test assumes
	beforeExport, err := source.Export.Export(ctx, sourceID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(beforeExport.Blockers) != 2 {
		t.Fatalf("source has %d blockers, want 2 (one open, one resolved)", len(beforeExport.Blockers))
	}
	if len(beforeExport.Checkpoints) == 0 || len(beforeExport.Memory) == 0 {
		t.Fatalf("source must carry at least one checkpoint and one memory entry")
	}

	data, err := source.Export.ExportJSON(ctx, sourceID)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	targetProjectID, err := target.ResolveProjectID(ctx, target.Root)
	if err != nil {
		t.Fatalf("resolve target project: %v", err)
	}
	importedID, err := target.Export.Import(ctx, targetProjectID, data, "claude")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	after, err := target.Export.Export(ctx, importedID)
	if err != nil {
		t.Fatalf("export the imported session: %v", err)
	}

	if after.Session.Status != entities.SessionStatusActive {
		t.Errorf("imported session status = %q, want %q regardless of the exported status",
			after.Session.Status, entities.SessionStatusActive)
	}
	if len(after.Tasks) != 1 {
		t.Fatalf("import produced %d tasks, want exactly 1", len(after.Tasks))
	}
	if after.Tasks[0].Status != entities.TaskStatusInProgress {
		t.Errorf("imported task status = %q, want %q (the export's %q must not survive)",
			after.Tasks[0].Status, entities.TaskStatusInProgress, entities.TaskStatusCompleted)
	}
	if len(after.Decisions) != 1 {
		t.Fatalf("import produced %d decisions, want exactly 1", len(after.Decisions))
	}
	if after.Decisions[0].Agent != "claude" {
		t.Errorf("imported decision agent = %q, want the importing agent %q, not the exporting %q",
			after.Decisions[0].Agent, "claude", "codex")
	}
	if len(after.Blockers) != 1 {
		t.Fatalf("import produced %d blockers, want exactly 1 (the open one; the resolved one must be dropped)", len(after.Blockers))
	}
	if after.Blockers[0].Description != openBlocker.Description {
		t.Errorf("imported blocker description = %q, want the open blocker's %q", after.Blockers[0].Description, openBlocker.Description)
	}
	for _, bl := range after.Blockers {
		if bl.Agent != "claude" {
			t.Errorf("imported blocker agent = %q, want the importing agent %q", bl.Agent, "claude")
		}
	}
	if len(after.Checkpoints) != 0 {
		t.Errorf("import restored %d checkpoints; export-v1.md says checkpoints are never read on import", len(after.Checkpoints))
	}
	if len(after.Memory) != 0 {
		t.Errorf("import restored %d memory entries; export-v1.md says memory is never read on import", len(after.Memory))
	}
	if len(after.Events) != 1 || after.Events[0].Type != entities.EventSessionStarted || after.Events[0].Agent != "claude" {
		t.Errorf("imported events = %v, want exactly the one session.started event Import itself writes", after.Events)
	}
}
