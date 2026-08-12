package services_test

import (
	"context"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// TestTestsStateLatestRun verifies the snapshot test status reflects only the
// most recent run: a later pass must reset earlier failures to zero, and the
// failure count must only count consecutive failing runs since the last pass.
func TestTestsStateLatestRun(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	appendEvent := func(typ string) {
		t.Helper()
		if err := fx.app.Event.Append(ctx, sessionID, "claude", typ, ""); err != nil {
			t.Fatalf("append %s: %v", typ, err)
		}
	}

	// failures first, then a pass
	appendEvent(entities.EventTestFailed)
	appendEvent(entities.EventTestFailed)
	appendEvent(entities.EventTestPassed)

	snap, err := fx.app.Checkpoint.BuildSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snap.Tests.Status != "passed" {
		t.Fatalf("expected status passed, got %q", snap.Tests.Status)
	}
	if snap.Tests.Failures != 0 {
		t.Fatalf("expected failures reset to 0 after pass, got %d", snap.Tests.Failures)
	}

	// new failing run on top of the earlier pass
	appendEvent(entities.EventTestFailed)
	appendEvent(entities.EventTestFailed)

	snap, err = fx.app.Checkpoint.BuildSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snap.Tests.Status != "failed" {
		t.Fatalf("expected status failed, got %q", snap.Tests.Status)
	}
	if snap.Tests.Failures != 2 {
		t.Fatalf("expected 2 consecutive failures, got %d", snap.Tests.Failures)
	}
}

// TestDiffDetectsResolvedBlocker verifies that resolving a blocker between two
// checkpoints is surfaced in the diff, even though the snapshot stores open
// blockers only.
func TestDiffDetectsResolvedBlocker(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	// open a blocker and checkpoint
	bl, err := fx.app.Decision.CreateBlocker(ctx, sessionID, "refresh token rotation", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	cp1, err := fx.app.Checkpoint.Create(ctx, sessionID, "before", "", "claude")
	if err != nil {
		t.Fatalf("checkpoint before: %v", err)
	}

	// resolve the blocker and checkpoint again
	if err := fx.app.Decision.ResolveBlocker(ctx, bl.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}
	cp2, err := fx.app.Checkpoint.Create(ctx, sessionID, "after", "", "claude")
	if err != nil {
		t.Fatalf("checkpoint after: %v", err)
	}

	diff, err := fx.app.Checkpoint.Diff(ctx, cp1.ID, cp2.ID)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.ResolvedBlockers) != 1 {
		t.Fatalf("expected 1 resolved blocker in diff, got %d", len(diff.ResolvedBlockers))
	}
	if diff.ResolvedBlockers[0].ID != bl.ID {
		t.Fatalf("expected resolved blocker %s, got %s", bl.ID, diff.ResolvedBlockers[0].ID)
	}
}
