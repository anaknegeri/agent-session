package services_test

import (
	"context"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	mdc "github.com/anaknegeri/agent-session/internal/infrastructure/context"
)

// smallBudget mimics a tight config to exercise truncation.
func smallBudget() ports.ContextBudget {
	b := ports.DefaultContextBudget()
	b.MaxDecisions = 2
	b.MaxBlockers = 1
	b.MaxFiles = 1
	b.MaxProgress = 2
	b.MaxItemChars = 20
	return b
}

func TestContextBudgetTruncates(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := fx.app.Decision.Create(ctx, initRes.Session.ID, "decision number "+string(rune('a'+i)), "", "claude"); err != nil {
			t.Fatal(err)
		}
	}
	// 5 completed tasks → progress.completed has 5, budget.MaxProgress=2
	var task *entities.Task
	for i := 0; i < 5; i++ {
		task, err = fx.app.Task.Create(ctx, initRes.Session.ID, "subtask "+string(rune('a'+i)), "claude")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fx.app.Task.Update(ctx, task.ID, "", entities.TaskStatusCompleted, "claude"); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := fx.app.Checkpoint.BuildSnapshot(ctx, initRes.Session.ID)
	if err != nil {
		t.Fatal(err)
	}

	text, err := mdc.NewRenderer().RenderContext(snapshot, smallBudget())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "+3 more decisions") {
		t.Fatalf("expected decision truncation note, got:\n%s", text)
	}
	if !strings.Contains(text, "+3 more") {
		t.Fatalf("expected progress truncation note, got:\n%s", text)
	}
	// every truncation must tell the agent how to get the full state
	if !strings.Contains(text, "depth=full") {
		t.Fatalf("expected fetch-more hint, got:\n%s", text)
	}
}

func TestFullDepthNeverClamped(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := fx.app.Decision.Create(ctx, initRes.Session.ID, "decision number "+string(rune('a'+i)), "", "claude"); err != nil {
			t.Fatal(err)
		}
	}

	tiny := ports.DefaultContextBudget()
	tiny.MaxTotalChars = 120
	cs := app.NewContextService(fx.app.Store, fx.app.Checkpoint, mdc.NewRenderer(), tiny)

	summary, err := cs.Get(ctx, initRes.Session.ID, "summary")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "summary clamped") {
		t.Fatalf("expected summary to be clamped with a hint, got:\n%s", summary)
	}

	full, err := cs.Get(ctx, initRes.Session.ID, "full")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(full, "summary clamped") {
		t.Fatalf("full depth must never be clamped:\n%s", full)
	}
	if !strings.Contains(full, "decision number j") {
		t.Fatalf("full depth should include all decisions:\n%s", full)
	}
}

func TestMemoryInjectionInSummary(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := fx.app.Task.Create(ctx, initRes.Session.ID, "Fix OAuth refresh token rotation", "claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.Memory.Put(ctx, initRes.Session.ID, entities.KnowledgeKindProject, "refresh tokens must rotate every session", "claude"); err != nil {
		t.Fatal(err)
	}

	text, err := fx.app.Context.Get(ctx, initRes.Session.ID, "summary")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "## Relevant memory") {
		t.Fatalf("expected relevant memory section, got:\n%s", text)
	}
	if !strings.Contains(text, "rotate every session") {
		t.Fatalf("expected memory content injected:\n%s", text)
	}
}
