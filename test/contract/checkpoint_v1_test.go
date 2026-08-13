package contract_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/contract"
)

// snapshotV1 is every JSON path a Checkpoint Schema v1 snapshot contains, with
// the type a reader will find there. See docs/spec/checkpoint-v1.md.
//
// A checkpoint is the one artefact that outlives the build that wrote it: the
// context an agent resumes from, `session.diff` and `restore` all read snapshots
// written weeks earlier by another version. So a rename or a type change here is
// not a refactor, it is a break in a stored format.
var snapshotV1 = []string{
	"blockers[].agent string",
	"blockers[].created_at string",
	"blockers[].description string",
	"blockers[].id string",
	"blockers[].resolved_at string",
	"blockers[].session_id string",
	"blockers[].status string",
	"decisions[].agent string",
	"decisions[].created_at string",
	"decisions[].decision string",
	"decisions[].id string",
	"decisions[].reason string",
	"decisions[].session_id string",
	"files.modified[] string",
	"last_agent string",
	"next_action string",
	"nudges[] string",
	"progress.completed[] string",
	"progress.pending[] string",
	"progress.tasks[].id string",
	"progress.tasks[].status string",
	"progress.tasks[].title string",
	"session.id string",
	"session.status string",
	"session.title string",
	"task.id string",
	"task.status string",
	"task.title string",
	"tests.failures int",
	"tests.status string",
	"version int",
	"workspace.branch string",
	"workspace.commit string",
	"workspace.dirty bool",
	"workspace.repository string",
}

// checkpointRowV1 is the JSON shape of a checkpoint itself, as returned by
// session.checkpoint and the session://checkpoint/latest resource.
var checkpointRowV1 = []string{
	"agent string",
	"created_at string",
	"id string",
	"kind string",
	"label string",
	"next_action string",
	"session_id string",
	"snapshot string",
	"task_id string",
}

// TestCheckpointSnapshotShapeV1 freezes the snapshot JSON. The failure names both
// directions because they differ: a path that disappeared breaks readers of
// existing checkpoints, a new one does not.
func TestCheckpointSnapshotShapeV1(t *testing.T) {
	got := jsonShape(reflect.TypeOf(entities.Snapshot{}))
	assertShape(t, "Checkpoint snapshot", "docs/spec/checkpoint-v1.md", snapshotV1, got)
}

func TestCheckpointRowShapeV1(t *testing.T) {
	got := jsonShape(reflect.TypeOf(entities.Checkpoint{}))
	assertShape(t, "Checkpoint", "docs/spec/checkpoint-v1.md", checkpointRowV1, got)
}

// TestSnapshotKeysAlwaysPresentV1 holds the one clause the shape tests structurally
// cannot see. jsonShape walks struct tags and jsonName drops everything after the
// comma, so `omitempty` is invisible to it — and `omitempty` is exactly what
// decides whether a key is there. checkpoint-v1.md: "`nudges` is omitted when
// empty. Every other key is always present; empty means empty string, zero or an
// empty array, never absent."
//
// The reader this protects is the one the spec is written for: something in another
// language reading `.agent/session.db`, or the model receiving a checkpoint from
// session.checkpoint, told to expect an array and handed `null` or nothing.
func TestSnapshotKeysAlwaysPresentV1(t *testing.T) {
	data, err := json.Marshal(entities.Snapshot{})
	if err != nil {
		t.Fatalf("marshal empty snapshot: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("empty snapshot is not JSON: %v", err)
	}

	for _, key := range topLevelKeysV1() {
		if key == "nudges" {
			if _, ok := raw[key]; ok {
				t.Errorf("`nudges` renders when empty; v1 says it is the one omitted key:\n%s", data)
			}
			continue
		}
		if _, ok := raw[key]; !ok {
			t.Errorf("an empty snapshot omits %q, so a reader told it is always present finds nothing\n see docs/spec/checkpoint-v1.md\n%s", key, data)
		}
	}

	// Nested objects are values, not optional sections: a snapshot with no task
	// still has to carry `task.id`, or "empty means empty string" is not true.
	for _, path := range []string{
		"session.id", "session.title", "session.status",
		"workspace.repository", "workspace.branch", "workspace.commit", "workspace.dirty",
		"task.id", "task.title", "task.status",
		"tests.status", "tests.failures",
	} {
		parent, key, _ := strings.Cut(path, ".")
		object, ok := raw[parent].(map[string]any)
		if !ok {
			t.Errorf("%q is not an object in an empty snapshot: %v", parent, raw[parent])
			continue
		}
		if _, ok := object[key]; !ok {
			t.Errorf("an empty snapshot omits %q\n see docs/spec/checkpoint-v1.md\n%s", path, data)
		}
	}

	// `null` is not an empty array. The stores hand back nil slices for "nothing
	// recorded yet", so this is the difference the marshaller has to erase.
	for _, path := range []string{
		"decisions", "blockers",
		"progress.completed", "progress.pending", "progress.tasks",
		"files.modified",
	} {
		value := lookupPath(t, raw, path)
		list, ok := value.([]any)
		if !ok {
			t.Errorf("%q is %v in an empty snapshot, want an empty array\n%s", path, value, data)
			continue
		}
		if len(list) != 0 {
			t.Errorf("%q is not empty in an empty snapshot: %v", path, list)
		}
	}
}

// topLevelKeysV1 derives the expected top-level key set from the frozen shape, so
// a field added to the snapshot has to be added to snapshotV1 (and to the spec)
// before this test can pass — the same gate the shape tests apply.
func topLevelKeysV1() []string {
	seen := map[string]bool{}
	keys := make([]string, 0, len(snapshotV1))
	for _, entry := range snapshotV1 {
		path, _, _ := strings.Cut(entry, " ")
		top, _, _ := strings.Cut(path, ".")
		top = strings.TrimSuffix(top, "[]")
		if seen[top] {
			continue
		}
		seen[top] = true
		keys = append(keys, top)
	}
	sort.Strings(keys)
	return keys
}

func lookupPath(t *testing.T, raw map[string]any, path string) any {
	t.Helper()
	parent, key, nested := strings.Cut(path, ".")
	if !nested {
		return raw[parent]
	}
	object, ok := raw[parent].(map[string]any)
	if !ok {
		t.Fatalf("%q is not an object: %v", parent, raw[parent])
	}
	return object[key]
}

// TestCheckpointKindsV1 freezes the kind vocabulary. Retention is applied per
// kind, so an unknown kind is unbounded growth and a renamed one silently loses
// the limit that was protecting the deliberate checkpoints.
func TestCheckpointKindsV1(t *testing.T) {
	want := []string{"auto", "handoff", "manual", "precompact"}
	got := append([]string(nil), entities.CheckpointKinds...)
	sort.Strings(got)
	if !reflect.DeepEqual(want, got) {
		t.Errorf("checkpoint kinds changed: want %v, got %v\n see docs/spec/checkpoint-v1.md", want, got)
	}
}

// TestCheckpointCarriesSchemaVersion is what makes the version worth having: a
// checkpoint written today has to say which schema it follows, or a future reader
// is back to guessing.
func TestCheckpointCarriesSchemaVersion(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()

	sessionID := activeSession(t, app)
	cp, err := app.Checkpoint.Create(ctx, sessionID, "contract", "", "claude")
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(cp.Snapshot), &raw); err != nil {
		t.Fatalf("stored snapshot is not JSON: %v", err)
	}
	version, ok := raw["version"]
	if !ok {
		t.Fatalf("the stored snapshot has no version field:\n%s", cp.Snapshot)
	}
	if got := int(version.(float64)); got != contract.Checkpoint {
		t.Errorf("snapshot declares version %d, this build writes v%d", got, contract.Checkpoint)
	}
}

// TestParseSnapshotAcceptsPreVersionedSnapshot covers the checkpoints already on
// disk. They predate the version field, and v1 is defined to be their shape, so
// reading one must succeed and report v1 rather than 0.
func TestParseSnapshotAcceptsPreVersionedSnapshot(t *testing.T) {
	app := newProject(t)

	cp := &entities.Checkpoint{
		ID:       "chk_prev1",
		Snapshot: `{"session":{"id":"sess_x","title":"old","status":"active"},"next_action":"continue"}`,
	}
	snap, err := app.Checkpoint.ParseSnapshot(cp)
	if err != nil {
		t.Fatalf("a pre-v1 snapshot must still be readable: %v", err)
	}
	if snap.Version != contract.Checkpoint {
		t.Errorf("pre-v1 snapshot reports version %d, want %d", snap.Version, contract.Checkpoint)
	}
	if snap.NextAction != "continue" {
		t.Errorf("next_action lost while parsing: %q", snap.NextAction)
	}
}

// TestParseSnapshotRejectsFutureSchema is the other half of the promise. Refusing
// is the point: silently rendering a shape this build does not understand hands
// the agent a context that may be wrong without saying so.
func TestParseSnapshotRejectsFutureSchema(t *testing.T) {
	app := newProject(t)

	cp := &entities.Checkpoint{
		ID:       "chk_future",
		Snapshot: fmt.Sprintf(`{"version":%d,"session":{"id":"sess_x"}}`, contract.Checkpoint+1),
	}
	if _, err := app.Checkpoint.ParseSnapshot(cp); err == nil {
		t.Fatal("a snapshot from a newer schema version was accepted")
	} else if !strings.Contains(err.Error(), "upgrade agent-session") {
		t.Errorf("the error should tell the user what to do, got: %v", err)
	}
}

func activeSession(t *testing.T, app *bootstrap.App) string {
	t.Helper()
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.GetActive(ctx, projectID)
	if err != nil {
		t.Fatalf("get active session: %v", err)
	}
	return session.ID
}

// assertShape compares a frozen JSON shape with the shipped one.
func assertShape(t *testing.T, what, spec string, want, got []string) {
	t.Helper()
	wantSet := make(map[string]bool, len(want))
	for _, p := range want {
		wantSet[p] = true
	}
	gotSet := make(map[string]bool, len(got))
	for _, p := range got {
		gotSet[p] = true
	}
	missing, extra := diffSets(wantSet, gotSet)
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	t.Errorf("%s no longer matches the v1 shape\n"+
		" gone or changed type (breaking, needs a version bump):\n   %s\n"+
		" new (compatible, add it to the baseline and %s):\n   %s",
		what, strings.Join(missing, "\n   "), spec, strings.Join(extra, "\n   "))
}

// jsonShape lists every JSON path in t with the type a reader finds there, so a
// shape can be compared as data instead of by eye. Slices render as `path[]`,
// nested objects as `parent.child`.
func jsonShape(rt reflect.Type) []string {
	var out []string
	walkJSON(rt, "", &out)
	sort.Strings(out)
	return out
}

func walkJSON(rt reflect.Type, prefix string, out *[]string) {
	for rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}

	// Timestamp marshals itself to an RFC 3339 string; walking its embedded
	// time.Time would describe the Go type instead of the JSON.
	if rt == reflect.TypeOf(entities.Timestamp{}) {
		*out = append(*out, strings.TrimSuffix(prefix, ".")+" string")
		return
	}

	switch rt.Kind() {
	case reflect.Struct:
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			if field.PkgPath != "" {
				continue // unexported: not in the JSON
			}
			name := jsonName(field)
			if name == "" {
				continue
			}
			walkJSON(field.Type, prefix+name+".", out)
		}
	case reflect.Slice, reflect.Array:
		walkJSON(rt.Elem(), strings.TrimSuffix(prefix, ".")+"[].", out)
	default:
		*out = append(*out, fmt.Sprintf("%s %s", strings.TrimSuffix(prefix, "."), rt.Kind()))
	}
}

func jsonName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	return name
}
