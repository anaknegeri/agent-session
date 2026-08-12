package services_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// TestHandoffRecordsStructuredEvent verifies the handoff event carries a full
// structured payload (handoff_id, from_agent, to_agent, checkpoint_id) so
// handoffs are auditable.
func TestHandoffRecordsStructuredEvent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	text, err := fx.app.Handoff.Handoff(ctx, sessionID, "codex")
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	if text == "" {
		t.Fatal("expected non-empty handoff text")
	}

	// find the handoff.created event
	events, err := fx.app.Event.List(ctx, sessionID, 50)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	for _, e := range events {
		if e.Type == entities.EventHandoffCreated {
			if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
				t.Fatalf("parse handoff payload: %v", err)
			}
			break
		}
	}
	if payload == nil {
		t.Fatal("handoff.created event not found")
	}
	for _, key := range []string{"handoff_id", "from_agent", "to_agent", "checkpoint_id"} {
		if payload[key] == "" {
			t.Fatalf("handoff payload missing %q: %+v", key, payload)
		}
	}
	if payload["from_agent"] != "claude" || payload["to_agent"] != "codex" {
		t.Fatalf("unexpected from/to agents: %+v", payload)
	}
}
