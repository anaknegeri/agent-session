package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
	"github.com/anaknegeri/agent-session/internal/wire"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

var errCheckpointRefused = errors.New("injected checkpoint failure")

// failingCheckpoints refuses to store a checkpoint, which cuts session.record
// after its event and its decision have already been written.
type failingCheckpoints struct {
	repositories.CheckpointRepository
}

func (failingCheckpoints) Create(context.Context, *entities.Checkpoint) error {
	return errCheckpointRefused
}

type refusingStore struct {
	ports.Store
}

func (s *refusingStore) Checkpoints() repositories.CheckpointRepository {
	return failingCheckpoints{CheckpointRepository: s.Store.Checkpoints()}
}

// Tx re-wraps the transaction-scoped store, or the injected failure would vanish
// the moment session.record opened its transaction — which is the path under test.
func (s *refusingStore) Tx(ctx context.Context, fn func(ports.Store) error) error {
	return s.Store.Tx(ctx, func(scoped ports.Store) error {
		return fn(&refusingStore{Store: scoped})
	})
}

// TestSessionRecordRollsBackWhenCheckpointFails is the regression test for the
// composite write that committed in three steps. The event, the decision and the
// checkpoint each had their own transaction, so a checkpoint failure returned an
// error over an event and a decision that were already stored — and the agent,
// told the call failed, retried it into a second copy of both.
//
// The failure is injected at the last of the three writes, which is the only place
// that tells the two designs apart: with separate transactions the first two
// survive.
func TestSessionRecordRollsBackWhenCheckpointFails(t *testing.T) {
	dir := t.TempDir()
	gitRepoForTest(t, dir)
	healthy, err := bootstrap.Init(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	projectID, err := healthy.ResolveProjectID(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := healthy.Session.GetActive(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}

	broken, err := wire.InitializeApp(&refusingStore{Store: healthy.Store}, logger.New("error"),
		ports.DefaultContextBudget(), healthy.Cfg.Retention)
	if err != nil {
		t.Fatal(err)
	}
	s := New(dir, logger.New("error"))
	s.app = &bootstrap.App{App: broken, Root: dir, Cfg: healthy.Cfg}

	c, err := client.NewInProcessClient(s.MCPServer())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}

	eventsBefore := len(sessionEvents(t, healthy, session.ID))

	req := mcp.CallToolRequest{}
	req.Params.Name = "session.record"
	req.Params.Arguments = map[string]any{
		"event_type":    entities.EventCommandExecuted,
		"event_payload": `{"cmd":"go test ./..."}`,
		"decision":      "neither of these may survive a failed checkpoint",
		"checkpoint":    true,
	}
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call session.record: %v", err)
	}
	if !res.IsError {
		t.Fatal("session.record reported success although the checkpoint was refused")
	}

	if after := len(sessionEvents(t, healthy, session.ID)); after != eventsBefore {
		t.Errorf("a failed session.record left %d new events committed", after-eventsBefore)
	}
	decisions, err := healthy.Decision.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	for _, d := range decisions {
		if strings.Contains(d.Decision, "may survive") {
			t.Errorf("a failed session.record committed its decision: %s", d.Decision)
		}
	}
}

// TestAutoCheckpointFailureDoesNotFailTheTool covers the four tools that returned
// the auto-checkpoint's error as their own. The task, decision or event has
// already committed by then, so an agent told the call failed retried it — and got
// a second task, a second decision, a second event. The auto-checkpoint is
// best-effort: it is logged and the call succeeds.
func TestAutoCheckpointFailureDoesNotFailTheTool(t *testing.T) {
	dir := t.TempDir()
	gitRepoForTest(t, dir)
	healthy, err := bootstrap.Init(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !healthy.Cfg.Session.AutoCheckpoint {
		t.Fatal("auto-checkpoint is off in this project, nothing to fail")
	}
	ctx := context.Background()

	broken, err := wire.InitializeApp(&refusingStore{Store: healthy.Store}, logger.New("error"),
		ports.DefaultContextBudget(), healthy.Cfg.Retention)
	if err != nil {
		t.Fatal(err)
	}
	s := New(dir, logger.New("error"))
	s.app = &bootstrap.App{App: broken, Root: dir, Cfg: healthy.Cfg}

	c, err := client.NewInProcessClient(s.MCPServer())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "task.create"
	req.Params.Arguments = map[string]any{"title": "recorded despite a refused checkpoint"}
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call task.create: %v", err)
	}
	if res.IsError {
		t.Fatalf("task.create failed because its auto-checkpoint did: %v", res.Content)
	}

	projectID, err := healthy.ResolveProjectID(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := healthy.Session.GetActive(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := healthy.Task.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("the project holds %d tasks, want the single created one", len(tasks))
	}
}

func sessionEvents(t *testing.T, app *bootstrap.App, sessionID string) []*entities.SessionEvent {
	t.Helper()
	events, err := app.Event.List(context.Background(), sessionID, 500)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return events
}
