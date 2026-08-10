package database

import (
	"context"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func TestSQLiteRoundTrip(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	project := &entities.Project{ID: "proj_1", Name: "demo", Path: "/demo"}
	if err := store.Projects().Create(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	got, err := store.Projects().GetByPath(ctx, "/demo")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if got.Name != "demo" || got.CreatedAt.IsZero() {
		t.Fatalf("round trip failed: %+v", got)
	}

	session := &entities.Session{
		ID:        "sess_1",
		ProjectID: "proj_1",
		Title:     "oauth",
		Status:    entities.SessionStatusActive,
		Branch:    "main",
	}
	if err := store.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	active, err := store.Sessions().GetActive(ctx, "proj_1")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active.ID != "sess_1" || active.UpdatedAt.IsZero() {
		t.Fatalf("session round trip failed: %+v", active)
	}

	decision := &entities.Decision{ID: "decision_1", SessionID: "sess_1", Decision: "rotate tokens"}
	if err := store.Decisions().Create(ctx, decision); err != nil {
		t.Fatalf("create decision: %v", err)
	}
	decisions, err := store.Decisions().ListBySession(ctx, "sess_1")
	if err != nil || len(decisions) != 1 {
		t.Fatalf("list decisions: %v, %d", err, len(decisions))
	}

	cp := &entities.Checkpoint{ID: "chk_1", SessionID: "sess_1", Snapshot: `{"ok":true}`}
	if err := store.Checkpoints().Create(ctx, cp); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	latest, err := store.Checkpoints().GetLatest(ctx, "sess_1")
	if err != nil || latest.ID != "chk_1" {
		t.Fatalf("get latest checkpoint: %v, %v", latest, err)
	}
}
