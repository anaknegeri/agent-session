package services_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func TestArtifactLargePayload(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// large payload (> threshold) must be stored as artifact reference
	big := strings.Repeat("x", 16*1024)
	if err := fx.app.Artifact.AppendEvent(ctx, initRes.Session.ID, "claude", entities.EventTestFailed, big); err != nil {
		t.Fatalf("append event: %v", err)
	}

	events, err := fx.app.Event.List(ctx, initRes.Session.ID, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (started + test), got %d", len(events))
	}
	last := events[0]
	if strings.Contains(last.Payload, "xxxxxxxx") {
		t.Fatalf("payload should not be stored inline for large input")
	}
	if !strings.Contains(last.Payload, `"artifact_id"`) {
		t.Fatalf("expected artifact reference, got: %s", last.Payload)
	}

	// small payload stays inline
	if err := fx.app.Artifact.AppendEvent(ctx, initRes.Session.ID, "claude", entities.EventTestPassed, `{"count":2}`); err != nil {
		t.Fatalf("append event: %v", err)
	}
	events, _ = fx.app.Event.List(ctx, initRes.Session.ID, 10)
	if !strings.Contains(events[0].Payload, `"count"`) {
		t.Fatalf("small payload should stay inline, got: %s", events[0].Payload)
	}
}

// TestArtifactPayloadCeiling covers the upper bound on an event payload. Without
// it, an agent piping a whole build log into event.append had the string in
// memory and then written into SQLite, and offloading it to an artifact moved the
// bytes without bounding them.
func TestArtifactPayloadCeiling(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	oversized := strings.Repeat("x", services.MaxPayloadSize+1)
	err = fx.app.Artifact.AppendEvent(ctx, initRes.Session.ID, "claude", entities.EventTestFailed, oversized)
	if err == nil {
		t.Fatalf("a %d byte payload was accepted over the %d byte limit", len(oversized), services.MaxPayloadSize)
	}
	// the error has to name both numbers, or the agent cannot tell how much to trim
	if !strings.Contains(err.Error(), strconv.Itoa(len(oversized))) || !strings.Contains(err.Error(), strconv.Itoa(services.MaxPayloadSize)) {
		t.Errorf("error names neither the size nor the limit: %v", err)
	}

	events, err := fx.app.Event.List(ctx, initRes.Session.ID, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("rejected payload still produced events: got %d, want 1 (session.started only)", len(events))
	}

	// the largest accepted payload still round-trips, so the ceiling is a ceiling
	// and not a narrower limit than it advertises
	atLimit := strings.Repeat("y", services.MaxPayloadSize)
	if err := fx.app.Artifact.AppendEvent(ctx, initRes.Session.ID, "claude", entities.EventTestFailed, atLimit); err != nil {
		t.Fatalf("payload exactly at the limit was rejected: %v", err)
	}
}

func TestHandoffCreatesCheckpoint(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := fx.app.Handoff.Handoff(ctx, initRes.Session.ID, "codex"); err != nil {
		t.Fatalf("handoff: %v", err)
	}

	latest, err := fx.app.Checkpoint.Latest(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("expected auto checkpoint after handoff: %v", err)
	}
	if latest.Label != "handoff" {
		t.Fatalf("expected handoff label, got %q", latest.Label)
	}
}
