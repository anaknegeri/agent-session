package services_test

import (
	"context"
	strings "strings"
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
		if err := fx.app.Artifact.AppendEvent(ctx, sessionID, "claude", typ, ""); err != nil {
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

// TestDiffDetectsResolvedBlockerAcrossCheckpoints verifies the open→resolved
// transition is still reported when the two checkpoints being compared are not
// adjacent: the resolution happened between cp1 and cp2, but the diff is taken
// between cp1 and cp3.
func TestDiffDetectsResolvedBlockerAcrossCheckpoints(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	bl, err := fx.app.Decision.CreateBlocker(ctx, sessionID, "refresh token rotation", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	cp1, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp1", "", "claude")
	if err != nil {
		t.Fatalf("cp1: %v", err)
	}

	if err := fx.app.Decision.ResolveBlocker(ctx, bl.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}
	if _, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp2", "", "claude"); err != nil {
		t.Fatalf("cp2: %v", err)
	}

	// nothing resolved between cp2 and cp3
	cp3, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp3", "", "claude")
	if err != nil {
		t.Fatalf("cp3: %v", err)
	}

	diff, err := fx.app.Checkpoint.Diff(ctx, cp1.ID, cp3.ID)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.ResolvedBlockers) != 1 {
		t.Fatalf("expected 1 resolved blocker across non-adjacent checkpoints, got %d", len(diff.ResolvedBlockers))
	}
	if diff.ResolvedBlockers[0].ID != bl.ID {
		t.Fatalf("expected resolved blocker %s, got %s", bl.ID, diff.ResolvedBlockers[0].ID)
	}
}

// TestSnapshotDoesNotAccumulateResolvedBlockers verifies a checkpoint snapshot
// never embeds blockers that were already resolved before it was taken. Carrying
// them makes every snapshot grow with the session's whole resolution history.
func TestSnapshotDoesNotAccumulateResolvedBlockers(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	bl, err := fx.app.Decision.CreateBlocker(ctx, sessionID, "stale blocker", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	if _, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp1", "", "claude"); err != nil {
		t.Fatalf("cp1: %v", err)
	}
	if err := fx.app.Decision.ResolveBlocker(ctx, bl.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}
	if _, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp2", "", "claude"); err != nil {
		t.Fatalf("cp2: %v", err)
	}
	cp3, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp3", "", "claude")
	if err != nil {
		t.Fatalf("cp3: %v", err)
	}

	stored, err := fx.app.Checkpoint.GetByID(ctx, cp3.ID)
	if err != nil {
		t.Fatalf("get cp3: %v", err)
	}
	if strings.Contains(stored.Snapshot, bl.ID) {
		t.Fatalf("cp3 snapshot still embeds blocker %s resolved two checkpoints earlier:\n%s", bl.ID, stored.Snapshot)
	}
}

// TestDiffDetectsTaskStatusTransition covers the review's §6 case: a task that
// becomes workable again (blocked → in_progress) must be reported as newly
// started. Both statuses are "not completed", so a diff based on pending-title
// lists cannot see the transition at all.
func TestDiffDetectsTaskStatusTransition(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	task, err := fx.app.Task.Create(ctx, sessionID, "rotate refresh tokens", "claude")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := fx.app.Task.Update(ctx, task.ID, "", entities.TaskStatusBlocked, "claude"); err != nil {
		t.Fatalf("set blocked: %v", err)
	}
	cp1, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp1", "", "claude")
	if err != nil {
		t.Fatalf("cp1: %v", err)
	}

	if _, err := fx.app.Task.Update(ctx, task.ID, "", entities.TaskStatusInProgress, "claude"); err != nil {
		t.Fatalf("set in_progress: %v", err)
	}
	cp2, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp2", "", "claude")
	if err != nil {
		t.Fatalf("cp2: %v", err)
	}

	diff, err := fx.app.Checkpoint.Diff(ctx, cp1.ID, cp2.ID)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	if len(diff.TaskTransitions) != 1 {
		t.Fatalf("expected 1 task transition, got %d: %+v", len(diff.TaskTransitions), diff.TaskTransitions)
	}
	tr := diff.TaskTransitions[0]
	if tr.TaskID != task.ID {
		t.Errorf("transition task id = %s, want %s", tr.TaskID, task.ID)
	}
	if tr.From != entities.TaskStatusBlocked || tr.To != entities.TaskStatusInProgress {
		t.Errorf("transition = %s → %s, want %s → %s",
			tr.From, tr.To, entities.TaskStatusBlocked, entities.TaskStatusInProgress)
	}
	if len(diff.NewlyStarted) != 1 || diff.NewlyStarted[0] != task.Title {
		t.Errorf("NewlyStarted = %v, want [%s]", diff.NewlyStarted, task.Title)
	}
	if !diff.HasChanges() {
		t.Error("HasChanges() = false for a task that started")
	}
}

// TestDiffCancelledTaskIsNotStarted verifies a cancelled task is not reported as
// newly started. It is not completed either, so a pending-list diff would treat
// it as ongoing work.
func TestDiffCancelledTaskIsNotStarted(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	cp1, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp1", "", "claude")
	if err != nil {
		t.Fatalf("cp1: %v", err)
	}

	task, err := fx.app.Task.Create(ctx, sessionID, "drop legacy sync", "claude")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := fx.app.Task.Update(ctx, task.ID, "", entities.TaskStatusCancelled, "claude"); err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	cp2, err := fx.app.Checkpoint.Create(ctx, sessionID, "cp2", "", "claude")
	if err != nil {
		t.Fatalf("cp2: %v", err)
	}

	diff, err := fx.app.Checkpoint.Diff(ctx, cp1.ID, cp2.ID)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	for _, title := range diff.NewlyStarted {
		if title == task.Title {
			t.Errorf("cancelled task %q reported as newly started", task.Title)
		}
	}
	for _, title := range diff.NewlyCompleted {
		if title == task.Title {
			t.Errorf("cancelled task %q reported as newly completed", task.Title)
		}
	}
}
