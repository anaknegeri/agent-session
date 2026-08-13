package mcp_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	agentsession "github.com/anaknegeri/agent-session/internal/infrastructure/mcp"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

func setupMCP(t *testing.T) (*client.Client, *bootstrap.App) {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	app, err := bootstrap.Init(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}

	server := agentsession.New(dir, logger.New("error"))
	c, err := client.NewInProcessClient(server.MCPServer())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	return c, app
}

func call(t *testing.T, c *client.Client, name string, args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("call %s returned error: %v", name, res.Content)
	}
	return toolText(res)
}

// callAny returns the tool output even when the tool signals an error.
func callAny(t *testing.T, c *client.Client, name string, args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return toolText(res)
}

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t.co")
	run("config", "user.name", "t")
	if err := writeFile(filepath.Join(dir, "server.go"), "package oauth\n"); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "init")
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestMCPSessionFlow(t *testing.T) {
	c, _ := setupMCP(t)
	ctx := context.Background()

	res := call(t, c, "decision.create", map[string]any{
		"decision": "Use rotating refresh tokens",
		"reason":   "Prevent token replay",
	})
	if !strings.Contains(res, "decision_") {
		t.Fatalf("decision.create unexpected: %s", res)
	}

	res = call(t, c, "session.checkpoint", map[string]any{
		"label":       "milestone-1",
		"next_action": "Fix refresh token rotation",
	})
	if !strings.Contains(res, "chk_") {
		t.Fatalf("session.checkpoint unexpected: %s", res)
	}

	res = call(t, c, "context.get", map[string]any{"depth": "summary"})
	if !strings.Contains(res, "rotating refresh tokens") {
		t.Fatalf("context.get missing decision: %s", res)
	}

	res = call(t, c, "workspace.status", nil)
	if !strings.Contains(res, "branch") {
		t.Fatalf("workspace.status unexpected: %s", res)
	}

	res = call(t, c, "session.resume", map[string]any{"agent": "opencode"})
	if !strings.Contains(res, "opencode") {
		t.Fatalf("session.resume unexpected: %s", res)
	}

	// resource read
	readRes, err := c.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "session://context"},
	})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(readRes.Contents) == 0 {
		t.Fatalf("expected resource contents")
	}

	_ = ctx
}

func TestMCPTaskAndBlockerTools(t *testing.T) {
	c, app := setupMCP(t)
	ctx := context.Background()

	// task.create
	res := call(t, c, "task.create", map[string]any{"title": "Implement OAuth2 PKCE"})
	if !strings.Contains(res, "task_") {
		t.Fatalf("task.create unexpected: %s", res)
	}

	res = call(t, c, "task.get", nil)
	if !strings.Contains(res, "OAuth2 PKCE") {
		t.Fatalf("task.get should return current task: %s", res)
	}

	// task.update to completed
	res = call(t, c, "task.update", map[string]any{"task_id": extractID(t, res), "status": "completed"})
	if !strings.Contains(res, "completed") {
		t.Fatalf("task.update unexpected: %s", res)
	}

	// blocker.create + list
	res = call(t, c, "blocker.create", map[string]any{"description": "refresh token tests failing"})
	if !strings.Contains(res, "blocker_") {
		t.Fatalf("blocker.create unexpected: %s", res)
	}
	res = call(t, c, "blocker.list", nil)
	if !strings.Contains(res, "refresh token tests failing") {
		t.Fatalf("blocker.list unexpected: %s", res)
	}

	// auto-checkpoint: after task.create a checkpoint should exist
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := app.Session.GetActive(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := app.Checkpoint.Latest(ctx, session.ID)
	if err != nil {
		t.Fatalf("expected auto checkpoint after task.create: %v", err)
	}
	if !strings.Contains(latest.Label, "task") {
		t.Fatalf("expected task label in auto checkpoint, got %q", latest.Label)
	}
}

func TestMCPMemoryTools(t *testing.T) {
	c, _ := setupMCP(t)

	res := call(t, c, "memory.put", map[string]any{
		"kind":    "architecture",
		"content": "Use rotating refresh tokens to prevent replay",
	})
	if !strings.Contains(res, "mem_") {
		t.Fatalf("memory.put unexpected: %s", res)
	}
	memID := extractIDLike(t, res, "mem_")

	res = call(t, c, "memory.search", map[string]any{"query": "refresh token"})
	if !strings.Contains(res, "rotating") {
		t.Fatalf("memory.search unexpected: %s", res)
	}

	res = call(t, c, "memory.get", map[string]any{"memory_id": memID})
	if !strings.Contains(res, "refresh") {
		t.Fatalf("memory.get unexpected: %s", res)
	}

	res = call(t, c, "memory.delete", map[string]any{"memory_id": memID})
	if !strings.Contains(res, "deleted") {
		t.Fatalf("memory.delete unexpected: %s", res)
	}
}

func TestMCPLazyInitAfterInit(t *testing.T) {
	// server starts before the project is initialized; it must recover once
	// agent-session init completes (always-on user-scope scenario).
	dir := t.TempDir()
	gitInit(t, dir)

	server := agentsession.New(dir, logger.New("error"))
	c, err := client.NewInProcessClient(server.MCPServer())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}

	// before init: tool returns the not-initialized message, still connected
	res := callAny(t, c, "session.get", nil)
	if !strings.Contains(res, "not initialized") {
		t.Fatalf("expected not initialized, got: %s", res)
	}

	// init the project, then the same server instance must work
	if _, err := bootstrap.Init(dir, "claude"); err != nil {
		t.Fatalf("init: %v", err)
	}
	res = call(t, c, "session.get", nil)
	if !strings.Contains(res, "sess_") {
		t.Fatalf("expected session after init, got: %s", res)
	}
}

func TestMCPContextSummarize(t *testing.T) {
	c, _ := setupMCP(t)

	res := call(t, c, "context.summarize", nil)
	if !strings.Contains(res, "memory.put") {
		t.Fatalf("context.summarize should instruct storing via memory.put, got: %s", res)
	}
	if !strings.Contains(res, "Agent Session") {
		t.Fatalf("context.summarize should include session data, got: %s", res)
	}
}

func TestMCPToolAnnotations(t *testing.T) {
	c, _ := setupMCP(t)

	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]mcp.Tool{}
	for _, tl := range res.Tools {
		byName[tl.Name] = tl
	}

	if tl, ok := byName["session.get"]; !ok || tl.Annotations.ReadOnlyHint == nil || !*tl.Annotations.ReadOnlyHint {
		t.Fatalf("session.get should be readOnly, got %+v", byName["session.get"].Annotations)
	}
	if tl, ok := byName["memory.delete"]; !ok || tl.Annotations.DestructiveHint == nil || !*tl.Annotations.DestructiveHint {
		t.Fatalf("memory.delete should be destructive, got %+v", byName["memory.delete"].Annotations)
	}
	if tl, ok := byName["memory.search"]; !ok || tl.Annotations.ReadOnlyHint == nil || !*tl.Annotations.ReadOnlyHint {
		t.Fatalf("memory.search should be readOnly, got %+v", byName["memory.search"].Annotations)
	}
}

// TestMCPInstructions ensures the server declares the `instructions` field in
// its InitializeResult. Clients like Claude Code surface this proactively as
// "MCP Server Instructions" in every turn, which is how agents discover the
// session workflow without being prompted.
func TestMCPInstructions(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	if _, err := bootstrap.Init(dir, "claude"); err != nil {
		t.Fatal(err)
	}

	server := agentsession.New(dir, logger.New("error"))
	c, err := client.NewInProcessClient(server.MCPServer())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	initRes, err := c.Initialize(context.Background(), mcp.InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if initRes.Instructions == "" {
		t.Fatal("InitializeResult.Instructions is empty; agents won't proactively learn about the session workflow")
	}
	for _, want := range []string{"session.get", "context.get", "session.checkpoint"} {
		if !strings.Contains(initRes.Instructions, want) {
			t.Fatalf("instructions missing %q", want)
		}
	}

	// The sandbox fallback has to name the symptom the client actually shows.
	// Codex under `approval: never` cancels every tool not annotated read-only
	// and reports `user cancelled MCP tool call`; an earlier wording said "if
	// your sandbox refuses tools that write", and the agent retried context.get
	// twice instead of falling back, because nothing it saw said "sandbox". The
	// exact string has to survive rewording, so it is asserted here.
	for _, want := range []string{"context.read", "user cancelled MCP tool call"} {
		if !strings.Contains(initRes.Instructions, want) {
			t.Fatalf("instructions missing %q, so a sandboxed client cannot find the read-only fallback", want)
		}
	}
}

func extractIDLike(t *testing.T, text, prefix string) string {
	t.Helper()
	start := strings.Index(text, prefix)
	if start < 0 {
		t.Fatalf("no id %q in %s", prefix, text)
	}
	end := start
	for end < len(text) && text[end] != '"' && text[end] != '\\' && text[end] != '}' {
		end++
	}
	return text[start:end]
}

func extractID(t *testing.T, text string) string {
	t.Helper()
	return extractIDLike(t, text, "task_")
}

// TestMCPSessionRecord verifies the unified session.record tool: it can append
// an event, create a decision, and create a checkpoint in a single call.
func TestMCPSessionRecord(t *testing.T) {
	c, app := setupMCP(t)

	res := call(t, c, "session.record", map[string]any{
		"event_type":      "test.passed",
		"event_payload":   `{"suite":"unit"}`,
		"decision":        "Use table-driven tests",
		"decision_reason": "Cleaner assertions",
		"next_action":     "Add more cases",
		"checkpoint":      "true",
	})
	if !strings.Contains(res, "decision_") || !strings.Contains(res, "chk_") {
		t.Fatalf("session.record output missing decision/checkpoint ids: %s", res)
	}

	session, err := app.Session.Get(ctxBackground(), appSessionID(app))
	if err != nil {
		t.Fatal(err)
	}
	events, _ := app.Event.List(ctxBackground(), session.ID, 50)
	hasTestPassed := false
	for _, e := range events {
		if e.Type == "test.passed" {
			hasTestPassed = true
			break
		}
	}
	if !hasTestPassed {
		t.Fatal("expected test.passed event recorded by session.record")
	}
}

func ctxBackground() context.Context { return context.Background() }

func appSessionID(app *bootstrap.App) string {
	projectID, err := app.ResolveProjectID(ctxBackground(), app.Root)
	if err != nil {
		return ""
	}
	s, err := app.Session.GetActive(ctxBackground(), projectID)
	if err != nil {
		return ""
	}
	return s.ID
}

// TestReadOnlyToolsDoNotWrite holds every tool that advertises readOnlyHint to
// what the hint claims. It is not decoration: Codex under `approval: never`
// executes read-only MCP tools and asks permission for the rest, so a tool that
// writes while claiming otherwise slips past the client's own gate.
//
// The check is a state fingerprint rather than a per-tool assertion, so a tool
// added later is covered without touching this test.
func TestReadOnlyToolsDoNotWrite(t *testing.T) {
	c, app := setupMCP(t)
	ctx := context.Background()

	// give the session something to read: a task, a decision and a dirty file, so
	// the tools take their populated paths rather than returning early on an empty
	// session — and so the auto-record path has something it would record
	call(t, c, "task.create", map[string]any{"title": "read-only audit"})
	call(t, c, "decision.create", map[string]any{"decision": "annotate honestly", "reason": "clients gate on it"})
	if err := writeFile(filepath.Join(app.Root, "changed.go"), "package oauth\n\n// dirty\n"); err != nil {
		t.Fatal(err)
	}

	res, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var readOnly []string
	for _, tl := range res.Tools {
		if tl.Annotations.ReadOnlyHint != nil && *tl.Annotations.ReadOnlyHint {
			readOnly = append(readOnly, tl.Name)
		}
	}
	if len(readOnly) == 0 {
		t.Fatal("no tool advertises readOnlyHint; the annotation wiring is broken")
	}

	for _, name := range readOnly {
		before := storeFingerprint(t, app)
		// callAny: a tool may legitimately fail for want of arguments, and a failed
		// call must still not have written anything
		callAny(t, c, name, nil)
		after := storeFingerprint(t, app)
		if before != after {
			t.Errorf("%s is annotated readOnly but changed the session state:\n before %s\n after  %s", name, before, after)
		}
	}
}

// storeFingerprint summarises everything a read-only tool must leave alone.
func storeFingerprint(t *testing.T, app *bootstrap.App) string {
	t.Helper()
	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.GetLatest(ctx, projectID)
	if err != nil {
		t.Fatalf("get latest session: %v", err)
	}
	events, err := app.Event.List(ctx, session.ID, 1000)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	checkpoints, err := app.Checkpoint.ListBySession(ctx, session.ID, 1000)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	tasks, err := app.Task.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	decisions, err := app.Decision.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	memories, err := app.Memory.ListByKind(ctx, "", 1000)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	return fmt.Sprintf("session=%s last_agent=%s task=%s events=%d checkpoints=%d tasks=%d decisions=%d memory=%d",
		session.ID, session.LastAgent, session.CurrentTaskID,
		len(events), len(checkpoints), len(tasks), len(decisions), len(memories))
}
