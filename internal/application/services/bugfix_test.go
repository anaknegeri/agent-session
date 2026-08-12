package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
)

func TestResolveBlockerNotFound(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	if _, err := fx.app.Init.Init(ctx, fx.dir, "claude"); err != nil {
		t.Fatalf("init: %v", err)
	}

	err := fx.app.Decision.ResolveBlocker(ctx, "blocker_does_not_exist")
	if !errors.Is(err, domainerr.ErrBlockerNotFound) {
		t.Fatalf("expected ErrBlockerNotFound, got %v", err)
	}
}

func TestTaskCreateRequiresTitle(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, err := fx.app.Task.Create(ctx, initRes.Session.ID, "   ", "claude"); !errors.Is(err, domainerr.ErrTaskTitleRequired) {
		t.Fatalf("expected ErrTaskTitleRequired, got %v", err)
	}
}

func TestAppendEventRejectsUnknownType(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	err = fx.app.Artifact.AppendEvent(ctx, initRes.Session.ID, "claude", "totally_made_up_type", "")
	if !errors.Is(err, domainerr.ErrInvalidEventType) {
		t.Fatalf("expected ErrInvalidEventType, got %v", err)
	}
}

func TestHandoffRejectsUnsupportedAgent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	_, err = fx.app.Handoff.Handoff(ctx, initRes.Session.ID, "nonexistent_agent")
	if !errors.Is(err, domainerr.ErrAgentNotSupported) {
		t.Fatalf("expected ErrAgentNotSupported, got %v", err)
	}
}

func TestHandoffTruncatesOversizedDecision(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	huge := strings.Repeat("x", 5000)
	if _, err := fx.app.Decision.Create(ctx, initRes.Session.ID, huge, "", "claude"); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	text, err := fx.app.Handoff.Handoff(ctx, initRes.Session.ID, "codex")
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	if strings.Contains(text, huge) {
		t.Fatalf("expected oversized decision to be truncated in handoff output, got full text of length %d", len(text))
	}
	if len(text) > 3000 {
		t.Fatalf("handoff output not bounded, got %d chars", len(text))
	}
}

func TestFullFlowUsesEventCanonicalTypes(t *testing.T) {
	// Guard against future canonical event types being added to entities.go
	// without registering them in IsCanonicalEventType.
	for _, ev := range []string{
		entities.EventSessionStarted, entities.EventTaskCreated, entities.EventTaskUpdated,
		entities.EventFileChanged, entities.EventCommandExecuted, entities.EventTestStarted,
		entities.EventTestFailed, entities.EventTestPassed, entities.EventDecisionCreated,
		entities.EventBlockerCreated, entities.EventCheckpointCreated, entities.EventHandoffCreated,
		entities.EventSessionCompleted,
	} {
		if !entities.IsCanonicalEventType(ev) {
			t.Fatalf("expected %s to be canonical", ev)
		}
	}
}
