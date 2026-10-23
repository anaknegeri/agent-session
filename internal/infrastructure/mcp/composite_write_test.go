package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// TestSessionRecordCommitsAllThree is the other half of
// TestSessionRecordRollsBackWhenCheckpointFails: the three writes still all land,
// and the results still name each one. A transaction that rolled everything back
// would satisfy the rollback test while recording nothing at all.
func TestSessionRecordCommitsAllThree(t *testing.T) {
	c, app := setupMCP(t)
	ctx := context.Background()
	sessionID := appSessionID(app)

	out := call(t, c, "session.record", map[string]any{
		"event_type":      entities.EventTestPassed,
		"event_payload":   `{"suite":"contract"}`,
		"decision":        "record in one call",
		"decision_reason": "three commits is three failure modes",
		"next_action":     "ship it",
		"checkpoint":      true,
	})

	var results struct {
		Event        string `json:"event"`
		DecisionID   string `json:"decision_id"`
		CheckpointID string `json:"checkpoint_id"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("session.record output is not JSON (%s): %v", out, err)
	}
	if results.Event != entities.EventTestPassed || results.DecisionID == "" || results.CheckpointID == "" {
		t.Fatalf("session.record did not report all three writes: %s", out)
	}

	cp, err := app.Checkpoint.GetByID(ctx, results.CheckpointID)
	if err != nil {
		t.Fatalf("the reported checkpoint does not resolve: %v", err)
	}
	if cp.NextAction != "ship it" {
		t.Errorf("checkpoint next_action is %q, want the recorded one", cp.NextAction)
	}
	found := false
	for _, ev := range listEvents(t, app, sessionID) {
		if ev.Type == entities.EventTestPassed {
			found = true
		}
	}
	if !found {
		t.Error("session.record did not append the event it reported")
	}
	decisions, err := app.Decision.List(ctx, sessionID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) == 0 {
		t.Error("session.record did not store the decision it reported")
	}
}

// TestArtifactGetResolvesOffloadedPayload covers the read path for an offloaded
// payload. Anything over 8192 bytes becomes {"artifact_id":"art_…"} in the event
// log, and nothing could resolve that id, so from a reader's point of view the
// payload was discarded.
func TestArtifactGetResolvesOffloadedPayload(t *testing.T) {
	c, app := setupMCP(t)
	sessionID := appSessionID(app)

	payload := `{"output":"` + strings.Repeat("y", 9000) + `"}`
	call(t, c, "event.append", map[string]any{
		"type":    entities.EventTestFailed,
		"payload": payload,
	})

	var artifactID string
	for _, ev := range listEvents(t, app, sessionID) {
		if ev.Type != entities.EventTestFailed {
			continue
		}
		var ref struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &ref); err != nil {
			t.Fatalf("event payload is not JSON: %v", err)
		}
		artifactID = ref.ArtifactID
	}
	if artifactID == "" {
		t.Fatal("a 9000 byte payload was not offloaded to an artifact")
	}

	out := call(t, c, "artifact.get", map[string]any{"artifact_id": artifactID})
	var artifact entities.Artifact
	if err := json.Unmarshal([]byte(out), &artifact); err != nil {
		t.Fatalf("artifact.get output is not JSON (%.120s): %v", out, err)
	}
	if artifact.Content != payload {
		t.Errorf("artifact.get returned %d bytes, want the %d byte payload", len(artifact.Content), len(payload))
	}

	if out := callAny(t, c, "artifact.get", map[string]any{"artifact_id": "art_does_not_exist"}); !strings.Contains(out, "artifact not found") {
		t.Errorf("artifact.get answered for an unknown id: %s", out)
	}
}

// TestToolsDoNotReturnNullCollections covers the shapes that crossed the MCP
// boundary as null or went missing entirely: session.diff answered
// "new_blockers":null and omitted task_transitions, memory.search answered a bare
// null, and workspace.status dropped changes on a clean tree — so a client had to
// treat absent, null and empty as the same thing on three different tools.
func TestToolsDoNotReturnNullCollections(t *testing.T) {
	c, _ := setupMCP(t)

	call(t, c, "session.checkpoint", map[string]any{"label": "first"})
	call(t, c, "session.checkpoint", map[string]any{"label": "second"})

	for _, tc := range []struct {
		tool string
		args map[string]any
		keys []string
	}{
		{
			tool: "session.diff",
			keys: []string{"new_decisions", "new_blockers", "resolved_blockers", "newly_completed", "newly_started", "task_transitions", "new_files"},
		},
		{
			tool: "workspace.status",
			keys: []string{"changes"},
		},
	} {
		out := call(t, c, tc.tool, tc.args)
		var row map[string]any
		if err := json.Unmarshal([]byte(out), &row); err != nil {
			t.Fatalf("%s output is not a JSON object (%s): %v", tc.tool, out, err)
		}
		for _, key := range tc.keys {
			value, ok := row[key]
			if !ok {
				t.Errorf("%s omits %q; an empty list is [], never absent", tc.tool, key)
				continue
			}
			if value == nil {
				t.Errorf("%s answers null for %q; an empty list is []", tc.tool, key)
			}
		}
	}

	// memory.search marshals the hit slice itself, so the whole result is the
	// collection: no match used to answer bare null.
	out := call(t, c, "memory.search", map[string]any{"query": "nothing matches this query"})
	if strings.TrimSpace(out) == "null" {
		t.Error("memory.search answers bare null for no hits; an empty result is []")
	}
	var hits []map[string]any
	if err := json.Unmarshal([]byte(out), &hits); err != nil {
		t.Fatalf("memory.search output is not a JSON array (%s): %v", out, err)
	}

	// A clean tree gives the git service a nil slice rather than an empty one, and
	// the fixture repository is never clean (it carries .agent/), so the nil path
	// is asserted on the value itself.
	clean, err := json.Marshal(ports.WorkspaceStatus{Branch: "main"})
	if err != nil {
		t.Fatalf("marshal workspace status: %v", err)
	}
	if !strings.Contains(string(clean), `"changes":[]`) {
		t.Errorf("a clean workspace marshals as %s; changes is [] when nothing changed", clean)
	}
}

func listEvents(t *testing.T, app *bootstrap.App, sessionID string) []*entities.SessionEvent {
	t.Helper()
	events, err := app.Event.List(context.Background(), sessionID, 500)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return events
}
