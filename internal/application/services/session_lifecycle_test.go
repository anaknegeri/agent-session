package services_test

import (
	"context"
	"testing"
)

func openAgentSessions(t *testing.T, f *fixture, sessionID string) int {
	t.Helper()
	rows, err := f.app.Store.AgentSessions().ListOpen(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("list open agent sessions: %v", err)
	}
	return len(rows)
}

// TestResumeClosesPreviousAgentSession verifies that resuming a session closes
// the previously open agent session before creating a new one, so exactly one
// agent session is ever active.
func TestResumeClosesPreviousAgentSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, err := f.app.Session.Resume(ctx, initRes.Session.ProjectID, "codex"); err != nil {
		t.Fatalf("resume 1: %v", err)
	}
	if _, err := f.app.Session.Resume(ctx, initRes.Session.ProjectID, "opencode"); err != nil {
		t.Fatalf("resume 2: %v", err)
	}

	if open := openAgentSessions(t, f, initRes.Session.ID); open != 1 {
		t.Fatalf("expected exactly 1 open agent session after resuming twice, got %d", open)
	}
}

// TestCompleteClosesAllAgentSessions verifies completing a session closes every
// open agent session (no ended_at=NULL leak).
func TestCompleteClosesAllAgentSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := f.app.Session.Resume(ctx, initRes.Session.ProjectID, "codex"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	if err := f.app.Session.Complete(ctx, initRes.Session.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if open := openAgentSessions(t, f, initRes.Session.ID); open != 0 {
		t.Fatalf("expected 0 open agent sessions after complete, got %d", open)
	}
}

// TestStartCompletesActiveSessionAndClosesAgent verifies that starting a new
// session completes the previous one and closes its open agent session.
func TestStartCompletesActiveSessionAndClosesAgent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	initRes, err := f.app.Init.Init(ctx, f.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	s2, err := f.app.Session.Start(ctx, initRes.Session.ProjectID, "second", "codex")
	if err != nil {
		t.Fatalf("start second session: %v", err)
	}

	// the first session's agent sessions must be closed
	if open := openAgentSessions(t, f, initRes.Session.ID); open != 0 {
		t.Fatalf("expected 0 open agent sessions on old session after new start, got %d", open)
	}
	// the new session has exactly one open agent session
	if open := openAgentSessions(t, f, s2.ID); open != 1 {
		t.Fatalf("expected 1 open agent session on new session, got %d", open)
	}
}
