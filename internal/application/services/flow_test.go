package services_test

import (
	"context"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func TestFullFlow(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	// UC-01: init
	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if initRes.Session.Status != entities.SessionStatusActive {
		t.Fatalf("expected active session, got %s", initRes.Session.Status)
	}
	if initRes.Session.Branch != "main" {
		t.Fatalf("expected branch main, got %q", initRes.Session.Branch)
	}

	// task + decision
	task, err := fx.app.Task.Create(ctx, initRes.Session.ID, "Implement OAuth2 PKCE", "claude")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID == "" {
		t.Fatalf("expected task id")
	}
	if _, err := fx.app.Decision.Create(ctx, initRes.Session.ID, "Use rotating refresh tokens", "Prevent token replay", "claude"); err != nil {
		t.Fatalf("create decision: %v", err)
	}
	if _, err := fx.app.Decision.CreateBlocker(ctx, initRes.Session.ID, "Refresh token rotation tests failing", "claude"); err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	// UC-03: checkpoint
	cp, err := fx.app.Checkpoint.Create(ctx, initRes.Session.ID, "", "Fix refresh token rotation", "claude")
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if !strings.Contains(cp.Snapshot, "Implement OAuth2 PKCE") {
		t.Fatalf("snapshot missing task title: %s", cp.Snapshot)
	}
	if !strings.Contains(cp.Snapshot, "rotating refresh tokens") {
		t.Fatalf("snapshot missing decision: %s", cp.Snapshot)
	}

	// snapshot reflects workspace (dirty after modification)
	writeFile(t, fx.dir, "server.go", "package oauth\n// dirty\n")
	snapshot, err := fx.app.Checkpoint.BuildSnapshot(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if !snapshot.Workspace.Dirty {
		t.Fatalf("expected dirty workspace, got %+v", snapshot.Workspace)
	}
	if len(snapshot.Files.Modified) == 0 {
		t.Fatalf("expected modified files, got none")
	}
	if len(snapshot.Blockers) != 1 {
		t.Fatalf("expected 1 open blocker, got %d", len(snapshot.Blockers))
	}

	// UC-04: handoff to codex
	text, err := fx.app.Handoff.Handoff(ctx, initRes.Session.ID, "codex")
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	for _, want := range []string{
		"Implement OAuth2 PKCE",
		"Use rotating refresh tokens",
		"Refresh token rotation tests failing",
		"server.go",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("handoff missing %q:\n%s", want, text)
		}
	}

	// UC-05: resume with opencode
	projectID, err := fx.app.ResolveProjectID(ctx, fx.dir)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	resumed, err := fx.app.Session.Resume(ctx, projectID, "opencode")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.ID != initRes.Session.ID {
		t.Fatalf("resume returned wrong session")
	}
	if resumed.LastAgent != "opencode" {
		t.Fatalf("expected last agent opencode, got %q", resumed.LastAgent)
	}

	// events recorded
	events, err := fx.app.Event.List(ctx, initRes.Session.ID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	types := map[string]bool{}
	for _, e := range events {
		types[e.Type] = true
	}
	for _, want := range []string{
		entities.EventSessionStarted,
		entities.EventTaskCreated,
		entities.EventDecisionCreated,
		entities.EventBlockerCreated,
		entities.EventCheckpointCreated,
		entities.EventHandoffCreated,
	} {
		if !types[want] {
			t.Fatalf("missing event %s in %v", want, types)
		}
	}
}

func TestSessionComplete(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := fx.app.Session.Complete(ctx, initRes.Session.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	session, err := fx.app.Session.Get(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if session.Status != entities.SessionStatusCompleted {
		t.Fatalf("expected completed, got %s", session.Status)
	}
}
