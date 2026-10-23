package contract_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	agentsession "github.com/anaknegeri/agent-session/internal/infrastructure/mcp"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// eventTypesV1 is the canonical event type namespace of Event Schema v1
// (docs/spec/event-v1.md).
//
// The event log is append-only and read by other agents and by `timeline`, so a
// type string is a name other programs match on. Renaming one does not migrate
// the events already written under the old name.
var eventTypesV1 = []string{
	"blocker.created",
	"blocker.resolved",
	"checkpoint.created",
	"command.executed",
	"decision.created",
	"file.changed",
	"handoff.created",
	"session.completed",
	"session.resumed",
	"session.started",
	"task.created",
	"task.updated",
	"test.failed",
	"test.passed",
	"test.started",
}

// eventRowV1 is the JSON shape of an event. Note `timestamp`, not `created_at`:
// the column is created_at and the JSON is not, and that mismatch is now frozen
// rather than waiting to be tidied into a break.
var eventRowV1 = []string{
	"agent string",
	"id string",
	"payload string",
	"session_id string",
	"timestamp string",
}

// payloadKeysV1 is the minimum set of keys each session-layer-produced event
// carries. A reader following a payload to the row it describes — the timeline
// resolving checkpoint_id, an audit resolving a handoff — depends on these.
//
// Types produced only from agent input (test.*, command.executed) are absent:
// their payload is whatever the agent passed, and v1 promises nothing about it
// beyond the artifact rule below.
var payloadKeysV1 = map[string][]string{
	"task.created":       {"task_id"},
	"task.updated":       {"status", "task_id"},
	"decision.created":   {"decision_id"},
	"blocker.created":    {"blocker_id"},
	"blocker.resolved":   {"blocker_id"},
	"checkpoint.created": {"checkpoint_id"},
	"handoff.created":    {"checkpoint_id", "from_agent", "handoff_id", "to_agent"},
	"file.changed":       {"files"},
}

func TestEventTypesV1(t *testing.T) {
	// Compared against the whole namespace, in both directions: a type added to
	// the code without being added here would otherwise ship undocumented, which
	// is how the table became a description of habit rather than a contract.
	want := append([]string(nil), eventTypesV1...)
	sort.Strings(want)
	if got := entities.CanonicalEventTypes(); !reflect.DeepEqual(want, got) {
		t.Errorf("the canonical event namespace no longer matches v1\n want %v\n got  %v\n see docs/spec/event-v1.md", want, got)
	}
	for _, extra := range []string{"session.paused", "memory.promoted", "workspace.synced"} {
		if entities.IsCanonicalEventType(extra) {
			t.Errorf("%q is canonical but not in the v1 baseline; add it here and to docs/spec/event-v1.md", extra)
		}
	}
}

func TestEventRowShapeV1(t *testing.T) {
	got := jsonShape(reflect.TypeOf(entities.SessionEvent{}))
	// type is a Go keyword-free field name but a reserved word in the shape
	// listing sense only; it is compared like any other.
	want := append(append([]string(nil), eventRowV1...), "type string")
	sort.Strings(want)
	assertShape(t, "SessionEvent", "docs/spec/event-v1.md", want, got)
}

// TestEventPayloadsV1 drives the real flows and reads back what they wrote. The
// payloads are built by string concatenation in several services, so asserting
// the shape from the outside is the only way to know they still agree with the
// spec.
func TestEventPayloadsV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	task, err := app.Task.Create(ctx, sessionID, "contract task", "claude")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := app.Task.Update(ctx, task.ID, "", entities.TaskStatusCompleted, "claude"); err != nil {
		t.Fatalf("update task: %v", err)
	}
	if _, err := app.Decision.Create(ctx, sessionID, "freeze the contract", "readers depend on it", "claude"); err != nil {
		t.Fatalf("create decision: %v", err)
	}
	blocker, err := app.Decision.CreateBlocker(ctx, sessionID, "waiting on review", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	if err := app.Decision.ResolveBlocker(ctx, blocker.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}
	if _, err := app.Checkpoint.Create(ctx, sessionID, "contract", "", "claude"); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if _, err := app.Handoff.Handoff(ctx, sessionID, "codex"); err != nil {
		t.Fatalf("handoff: %v", err)
	}

	events, err := app.Event.List(ctx, sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	seen := make(map[string]bool)
	for _, ev := range events {
		want, specified := payloadKeysV1[ev.Type]
		if !specified {
			continue
		}
		seen[ev.Type] = true

		var payload map[string]any
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Errorf("%s payload is not JSON (%q): %v", ev.Type, ev.Payload, err)
			continue
		}
		for _, key := range want {
			value, ok := payload[key]
			if !ok {
				t.Errorf("%s payload is missing %q: %s\n see docs/spec/event-v1.md", ev.Type, key, ev.Payload)
				continue
			}
			if s, isString := value.(string); isString && s == "" {
				t.Errorf("%s payload has an empty %q: %s", ev.Type, key, ev.Payload)
			}
		}
	}

	// file.changed is produced by the sync service from git, not by any call
	// above, so it is exercised separately rather than assumed.
	for _, eventType := range sortedStrings(payloadKeysV1) {
		if eventType == entities.EventFileChanged {
			continue
		}
		if !seen[eventType] {
			t.Errorf("no %s event was produced by the v1 flows; either the flow stopped recording it or the spec is stale", eventType)
		}
	}
}

// TestFileChangedPayloadV1 covers the one payload the flows above cannot produce.
func TestFileChangedPayloadV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	if err := app.Artifact.AppendEvent(ctx, sessionID, "claude", entities.EventFileChanged, `{"files":["a.go"]}`); err != nil {
		t.Fatalf("append file.changed: %v", err)
	}
	events, err := app.Event.List(ctx, sessionID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events {
		if ev.Type != entities.EventFileChanged {
			continue
		}
		var payload struct {
			Files []string `json:"files"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Fatalf("file.changed payload is not JSON: %v", err)
		}
		if len(payload.Files) == 0 {
			t.Errorf("file.changed carries no files: %s", ev.Payload)
		}
		return
	}
	t.Fatal("file.changed event not found")
}

// TestEmptyPayloadIsJSONV1 holds the one clause the spec states about a payload
// with nothing in it: it is `{}`, and the key is always present. Every event the
// session layer writes without a payload — session.started, session.resumed,
// session.completed — used to be stored as an empty string and marshalled with the
// key dropped entirely, so a reader in another language either parsed that as JSON
// or handled a missing field. 22 such rows exist in this repo's own database.
func TestEmptyPayloadIsJSONV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	if err := app.Artifact.AppendEvent(ctx, sessionID, "claude", entities.EventCommandExecuted, ""); err != nil {
		t.Fatalf("append with an empty payload: %v", err)
	}

	events, err := app.Event.List(ctx, sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no events recorded")
	}
	for _, ev := range events {
		if ev.Payload != entities.EmptyPayload && ev.Payload != "" {
			// a payload with content is checked by the tests above
			if !json.Valid([]byte(ev.Payload)) {
				t.Errorf("%s payload is not JSON: %q", ev.Type, ev.Payload)
			}
			continue
		}
		if ev.Payload == "" {
			t.Errorf("%s was stored with an empty payload; the append path normalizes it to %s", ev.Type, entities.EmptyPayload)
		}

		// what a client actually receives, which is where the key has to exist
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		var row map[string]any
		if err := json.Unmarshal(data, &row); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		payload, ok := row["payload"]
		if !ok {
			t.Errorf("%s marshalled without a payload key: %s", ev.Type, data)
			continue
		}
		if payload != entities.EmptyPayload {
			t.Errorf("%s marshalled payload %v, v1 promises %s when there is nothing to say", ev.Type, payload, entities.EmptyPayload)
		}
	}

	// A row written before the normalization existed still reads as `{}`, since
	// the historical rows are tolerated rather than migrated.
	historical := entities.SessionEvent{ID: "evt_old", SessionID: sessionID, Type: entities.EventSessionStarted}
	data, err := json.Marshal(historical)
	if err != nil {
		t.Fatalf("marshal historical event: %v", err)
	}
	if !strings.Contains(string(data), `"payload":"{}"`) {
		t.Errorf("a pre-normalization row does not render as %s: %s", entities.EmptyPayload, data)
	}
}

// TestLargePayloadBecomesArtifactRefV1 freezes the offload rule. A reader that
// finds artifact_id instead of the payload it expected has to know that is
// normal, and at which size it starts, or it will report the payload as lost.
func TestLargePayloadBecomesArtifactRefV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	if services.LargePayloadThreshold != 8*1024 {
		t.Errorf("the offload threshold is %d bytes, the spec says 8192", services.LargePayloadThreshold)
	}

	big := `{"output":"` + strings.Repeat("x", services.LargePayloadThreshold+1) + `"}`
	if err := app.Artifact.AppendEvent(ctx, sessionID, "claude", entities.EventTestFailed, big); err != nil {
		t.Fatalf("append large event: %v", err)
	}

	events, err := app.Event.List(ctx, sessionID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events {
		if ev.Type != entities.EventTestFailed {
			continue
		}
		var payload struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Fatalf("offloaded payload is not JSON: %v", err)
		}
		if payload.ArtifactID == "" {
			t.Fatalf("a payload over the threshold was stored inline instead of as an artifact: %d bytes", len(ev.Payload))
		}
		if !strings.HasPrefix(payload.ArtifactID, "art_") {
			t.Errorf("artifact_id %q does not use the art_ prefix", payload.ArtifactID)
		}
		return
	}
	t.Fatal("test.failed event not found")
}

// TestNonCanonicalEventTypeRejectedV1 keeps the namespace closed on every path an
// event can reach the log through. The check lived on one write path while a
// second, unvalidated one existed beside it, so "the namespace is closed" held
// only for callers that happened to pick the guarded path — and the type table
// above described habit rather than a contract.
//
// Each path is also driven with a canonical type: a path that rejects everything
// would satisfy the rejection assertion while accepting nothing at all.
func TestNonCanonicalEventTypeRejectedV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	// The service path. The CLI's `event add` and both MCP tools below funnel
	// through it, and it is the only append the application layer exposes.
	if err := app.Artifact.AppendEvent(ctx, sessionID, "claude", "made.up", `{}`); !errors.Is(err, domainerr.ErrInvalidEventType) {
		t.Errorf("service append accepted a non-canonical type, got err %v", err)
	}
	if err := app.Artifact.AppendEvent(ctx, sessionID, "claude", entities.EventCommandExecuted, `{}`); err != nil {
		t.Fatalf("service append rejected a canonical type: %v", err)
	}

	c := connect(t, agentsession.New(app.Root, logger.New("error")))
	for _, tool := range []struct {
		name    string
		typeArg string
	}{
		{"event.append", "type"},
		{"session.record", "event_type"},
	} {
		text, failed := callToolV1(t, c, tool.name, map[string]any{tool.typeArg: "made.up"})
		if !failed {
			t.Errorf("%s accepted a non-canonical event type: %s", tool.name, text)
		}
		if text, failed := callToolV1(t, c, tool.name, map[string]any{tool.typeArg: entities.EventCommandExecuted}); failed {
			t.Fatalf("%s rejected a canonical type: %s", tool.name, text)
		}
	}

	// A rejection that still writes the row would leave readers matching on a
	// type the table does not list, which is the damage the check exists to stop.
	events, err := app.Event.List(ctx, sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events {
		if !entities.IsCanonicalEventType(ev.Type) {
			t.Errorf("a rejected append still reached the log as %q", ev.Type)
		}
	}
}

// callToolV1 reports the tool output and whether the tool signalled failure. A
// tool error is carried in the result, not returned as a transport error.
func callToolV1(t *testing.T, c *client.Client, name string, args map[string]any) (string, bool) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var b strings.Builder
	for _, content := range res.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func sortedStrings(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
