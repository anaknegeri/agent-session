package services_test

import (
	"context"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// TestCheckpointRetentionIsPerKind verifies a burst of automatic checkpoints
// cannot evict the deliberate ones. Measured on a real session before this
// existed: 85 checkpoints, 17 of them from the Stop hook, with no bound at all.
func TestCheckpointRetentionIsPerKind(t *testing.T) {
	fx := newFixtureWithRetention(t, 3)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	// two deliberate checkpoints, then far more automatic ones than the limit
	manual1, err := fx.app.Checkpoint.CreateKind(ctx, sessionID, entities.CheckpointKindManual, "milestone one", "", "claude")
	if err != nil {
		t.Fatal(err)
	}
	manual2, err := fx.app.Checkpoint.CreateKind(ctx, sessionID, entities.CheckpointKindManual, "milestone two", "", "claude")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := fx.app.Checkpoint.CreateKind(ctx, sessionID, entities.CheckpointKindAuto, "auto", "", "claude"); err != nil {
			t.Fatal(err)
		}
	}

	cps, err := fx.app.Checkpoint.ListBySession(ctx, sessionID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]int{}
	ids := map[string]bool{}
	for _, cp := range cps {
		byKind[cp.Kind]++
		ids[cp.ID] = true
	}

	if byKind[entities.CheckpointKindAuto] > 3 {
		t.Errorf("auto checkpoints = %d, want at most 3", byKind[entities.CheckpointKindAuto])
	}
	if !ids[manual1.ID] || !ids[manual2.ID] {
		t.Errorf("deliberate checkpoints were evicted by the automatic burst: manual1=%v manual2=%v",
			ids[manual1.ID], ids[manual2.ID])
	}
}

// TestCheckpointRetentionKeepsLatest verifies pruning never removes the session's
// most recent checkpoint, which resume and restore depend on.
func TestCheckpointRetentionKeepsLatest(t *testing.T) {
	fx := newFixtureWithRetention(t, 1)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	var last *entities.Checkpoint
	for i := 0; i < 5; i++ {
		cp, err := fx.app.Checkpoint.CreateKind(ctx, sessionID, entities.CheckpointKindAuto, "auto", "keep going", "claude")
		if err != nil {
			t.Fatal(err)
		}
		last = cp
	}

	latest, err := fx.app.Checkpoint.Latest(ctx, sessionID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID != last.ID {
		t.Errorf("latest checkpoint = %s, want the most recent %s", latest.ID, last.ID)
	}
	if _, err := fx.app.Checkpoint.GetByID(ctx, last.ID); err != nil {
		t.Errorf("most recent checkpoint was pruned: %v", err)
	}
}

// TestCheckpointKindDerivedFromLabel covers hooks installed before the kind
// column existed: they pass only `--label auto` or `--label precompact`.
func TestCheckpointKindDerivedFromLabel(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	for label, want := range map[string]string{
		"auto":                    entities.CheckpointKindAuto,
		"auto-checkpoint (stale)": entities.CheckpointKindAuto,
		"precompact":              entities.CheckpointKindPreCompact,
		"handoff":                 entities.CheckpointKindHandoff,
		"v0.1.6 released":         entities.CheckpointKindManual,
		"":                        entities.CheckpointKindManual,
	} {
		cp, err := fx.app.Checkpoint.Create(ctx, sessionID, label, "", "claude")
		if err != nil {
			t.Fatalf("create %q: %v", label, err)
		}
		if cp.Kind != want {
			t.Errorf("label %q -> kind %q, want %q", label, cp.Kind, want)
		}
	}
}
