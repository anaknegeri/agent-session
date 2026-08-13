package contract_test

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	agentsession "github.com/anaknegeri/agent-session/internal/infrastructure/mcp"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// toolsV1 is the MCP Tool Contract v1 surface: every tool name a client may rely
// on, with the annotations it may gate its behaviour on.
//
// The annotations are load-bearing, not documentation. Codex under
// `approval: never` executes tools carrying readOnlyHint and raises a permission
// request for the rest, so flipping one changes which tools a real agent can call
// at all. See docs/spec/mcp-tools-v1.md.
var toolsV1 = map[string]annotations{
	"session.get":        {readOnly: true},
	"session.diff":       {readOnly: true},
	"session.checkpoint": {idempotent: true},
	"session.resume":     {idempotent: true},
	"session.record":     {idempotent: true},

	// context.get is idempotent, not read-only: it syncs file changes and may
	// auto-checkpoint. context.read is the read-only path a sandboxed client uses.
	"context.get":       {idempotent: true},
	"context.read":      {readOnly: true},
	"context.update":    {idempotent: true},
	"context.summarize": {readOnly: true},

	"task.create": {idempotent: true},
	"task.get":    {readOnly: true},
	"task.update": {idempotent: true},

	"decision.create": {idempotent: true},
	"decision.list":   {readOnly: true},

	"blocker.create":  {idempotent: true},
	"blocker.list":    {readOnly: true},
	"blocker.resolve": {idempotent: true},

	"event.append": {idempotent: true},

	"memory.put":     {idempotent: true},
	"memory.get":     {readOnly: true},
	"memory.search":  {readOnly: true},
	"memory.delete":  {destructive: true},
	"memory.promote": {idempotent: true},

	"workspace.status": {readOnly: true},
	"workspace.diff":   {readOnly: true},
}

// resourcesV1 is the MCP resource surface of the same contract.
var resourcesV1 = []string{
	"memory://recent",
	"session://checkpoint/latest",
	"session://context",
	"session://current",
	"session://decisions",
	"session://tasks",
	"session://workspace",
}

type annotations struct {
	readOnly    bool
	destructive bool
	idempotent  bool
}

func (a annotations) String() string {
	return fmt.Sprintf("readOnly=%t destructive=%t idempotent=%t", a.readOnly, a.destructive, a.idempotent)
}

// TestToolContractV1 holds the tool surface to the names and annotations v1
// promises. A rename, a removal or a flipped annotation breaks clients that are
// already wired to it; an addition does not, but still has to be added here and
// to the spec in the same commit, so nothing lands unnoticed.
func TestToolContractV1(t *testing.T) {
	tools := listTools(t)

	want := make(map[string]bool, len(toolsV1))
	for name := range toolsV1 {
		want[name] = true
	}
	got := make(map[string]bool, len(tools))
	for name := range tools {
		got[name] = true
	}

	if missing, extra := diffSets(want, got); len(missing) > 0 || len(extra) > 0 {
		t.Errorf("the MCP tool surface no longer matches Tool Contract v1\n"+
			" gone (breaking, needs a version bump): %v\n"+
			" new (compatible, add it to toolsV1 and docs/spec/mcp-tools-v1.md): %v",
			missing, extra)
	}

	for _, name := range sortedKeys(toolsV1) {
		want := toolsV1[name]
		got, ok := tools[name]
		if !ok {
			continue // already reported above
		}
		if got != want {
			t.Errorf("%s annotations changed: want %s, got %s\n"+
				" a client may gate tool calls on these; see docs/spec/mcp-tools-v1.md",
				name, want, got)
		}
	}
}

// TestToolCountV1 states the surface size on its own, so the number quoted in the
// README and in the spec has something holding it up.
func TestToolCountV1(t *testing.T) {
	if len(toolsV1) != 25 {
		t.Fatalf("the v1 baseline lists %d tools, the spec and README say 25", len(toolsV1))
	}
	if n := len(listTools(t)); n != 25 {
		t.Errorf("the server exposes %d tools, v1 is 25", n)
	}
}

// TestResourceContractV1 holds the resource URIs. A client bookmarks a URI, so a
// rename is as breaking as a renamed tool.
func TestResourceContractV1(t *testing.T) {
	app := newProject(t)
	server := agentsession.New(app.Root, logger.New("error"))
	c := connect(t, server)

	res, err := c.ListResources(context.Background(), mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}

	want := make(map[string]bool, len(resourcesV1))
	for _, uri := range resourcesV1 {
		want[uri] = true
	}
	got := make(map[string]bool, len(res.Resources))
	for _, r := range res.Resources {
		got[r.URI] = true
	}
	if missing, extra := diffSets(want, got); len(missing) > 0 || len(extra) > 0 {
		t.Errorf("the MCP resource surface no longer matches v1\n gone: %v\n new: %v\n"+
			" update resourcesV1 and docs/spec/mcp-tools-v1.md", missing, extra)
	}
}

// TestToolDescriptionsArePresent is the one quality bar v1 sets on the text: a
// tool with no description makes an agent guess, which is how the wrong tool gets
// called. The wording is free to change; its absence is not.
func TestToolDescriptionsArePresent(t *testing.T) {
	app := newProject(t)
	server := agentsession.New(app.Root, logger.New("error"))
	c := connect(t, server)

	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Description == "" {
			t.Errorf("%s has no description", tl.Name)
		}
	}
}

func listTools(t *testing.T) map[string]annotations {
	t.Helper()
	app := newProject(t)
	server := agentsession.New(app.Root, logger.New("error"))
	c := connect(t, server)

	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	out := make(map[string]annotations, len(res.Tools))
	for _, tl := range res.Tools {
		out[tl.Name] = annotations{
			readOnly:    isTrue(tl.Annotations.ReadOnlyHint),
			destructive: isTrue(tl.Annotations.DestructiveHint),
			idempotent:  isTrue(tl.Annotations.IdempotentHint),
		}
	}
	return out
}

func connect(t *testing.T, server *agentsession.Server) *client.Client {
	t.Helper()
	c, err := client.NewInProcessClient(server.MCPServer())
	if err != nil {
		t.Fatalf("in-process client: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

func isTrue(b *bool) bool { return b != nil && *b }

func sortedKeys(m map[string]annotations) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
