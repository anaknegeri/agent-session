package services_test

import (
	"context"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func TestMemoryPutSearchDelete(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	k, err := fx.app.Memory.Put(ctx, "", entities.KnowledgeKindArchitecture, "Use rotating refresh tokens to prevent replay", "claude")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if k.ID == "" {
		t.Fatalf("expected memory id")
	}

	got, err := fx.app.Memory.Get(ctx, k.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != k.Content {
		t.Fatalf("get mismatch: %s", got.Content)
	}

	hits, err := fx.app.Memory.Search(ctx, "refresh token", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected search hit for refresh token")
	}
	if !strings.Contains(hits[0].Content, "rotating") {
		t.Fatalf("unexpected hit: %+v", hits[0])
	}

	if err := fx.app.Memory.Delete(ctx, k.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	hits, _ = fx.app.Memory.Search(ctx, "refresh token", 10)
	for _, h := range hits {
		if h.ID == k.ID {
			t.Fatalf("deleted entry still searchable")
		}
	}
}

func TestMemoryPromoteIdempotent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sessionID := initRes.Session.ID

	// one decision, one completed task, one resolved blocker
	decision, err := fx.app.Decision.Create(ctx, sessionID, "Use rotating refresh tokens", "Prevent replay", "claude")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	task, err := fx.app.Task.Create(ctx, sessionID, "Implement PKCE validation", "claude")
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := fx.app.Task.Update(ctx, task.ID, "", entities.TaskStatusCompleted, "claude"); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	blocker, err := fx.app.Decision.CreateBlocker(ctx, sessionID, "Refresh token rotation tests failing", "claude")
	if err != nil {
		t.Fatalf("blocker: %v", err)
	}
	if err := fx.app.Decision.ResolveBlocker(ctx, blocker.ID); err != nil {
		t.Fatalf("resolve blocker: %v", err)
	}

	// promote: decision -> architecture, task -> solution, blocker -> project_knowledge
	count, err := fx.app.Memory.Promote(ctx, sessionID, "claude")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 promoted, got %d", count)
	}

	// idempotent: second promote adds nothing
	count, err = fx.app.Memory.Promote(ctx, sessionID, "claude")
	if err != nil {
		t.Fatalf("promote again: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 on re-promote, got %d", count)
	}

	// kinds present
	arch, err := fx.app.Memory.ListByKind(ctx, entities.KnowledgeKindArchitecture, 10)
	if err != nil {
		t.Fatalf("list architecture: %v", err)
	}
	if len(arch) != 1 || arch[0].SourceID != decision.ID {
		t.Fatalf("architecture promotion wrong: %+v", arch)
	}

	solutions, _ := fx.app.Memory.ListByKind(ctx, entities.KnowledgeKindSolution, 10)
	if len(solutions) != 1 || solutions[0].SourceID != task.ID {
		t.Fatalf("solution promotion wrong: %+v", solutions)
	}

	proj, _ := fx.app.Memory.ListByKind(ctx, entities.KnowledgeKindProject, 10)
	if len(proj) != 1 || proj[0].SourceID != blocker.ID {
		t.Fatalf("project_knowledge promotion wrong: %+v", proj)
	}
}
