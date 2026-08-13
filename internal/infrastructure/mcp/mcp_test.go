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
	"github.com/anaknegeri/agent-session/internal/domain/entities"
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

// TestResourcesDoNotWrite holds the resource surface to the promise the protocol
// makes for it: resources are read-only. session://context used to render through
// Context.Get, which syncs file changes and auto-checkpoints a stale session, so a
// client polling the resource — the cheap path clients are pointed at — was
// writing to the session on every poll.
func TestResourcesDoNotWrite(t *testing.T) {
	c, app := setupMCP(t)

	// give every resource something to return, and leave an unrecorded change in
	// the working tree: the sync and the auto-checkpoint only fire when git reports
	// files the session has not seen
	call(t, c, "task.create", map[string]any{"title": "resource audit"})
	call(t, c, "decision.create", map[string]any{"decision": "resources stay read-only", "reason": "clients poll them"})
	call(t, c, "memory.put", map[string]any{"kind": "project_knowledge", "content": "resources are read-only"})
	call(t, c, "session.checkpoint", map[string]any{"label": "before resource reads"})
	if err := writeFile(filepath.Join(app.Root, "polled.go"), "package oauth\n\n// unrecorded\n"); err != nil {
		t.Fatal(err)
	}

	for _, uri := range resourceURIs(t, c) {
		before := storeFingerprint(t, app)
		readResource(t, c, uri)
		readResource(t, c, uri)
		if after := storeFingerprint(t, app); before != after {
			t.Errorf("resource %s changed the session state:\n before %s\n after  %s", uri, before, after)
		}
	}
}

// TestResourcesEchoRequestedURI covers the URI a resource labels its contents
// with. Every handler returned a single hardcoded "session://resource", so a
// client reading memory://recent got contents attributed to something else and
// could not match a response to the request it made.
func TestResourcesEchoRequestedURI(t *testing.T) {
	c, _ := setupMCP(t)

	call(t, c, "task.create", map[string]any{"title": "uri audit"})
	call(t, c, "decision.create", map[string]any{"decision": "echo the uri", "reason": "clients match on it"})
	call(t, c, "memory.put", map[string]any{"kind": "project_knowledge", "content": "uris are echoed"})
	call(t, c, "session.checkpoint", map[string]any{"label": "before uri reads"})

	uris := resourceURIs(t, c)
	if len(uris) != 7 {
		t.Fatalf("expected 7 resources, got %d: %v", len(uris), uris)
	}
	for _, uri := range uris {
		res := readResource(t, c, uri)
		if len(res.Contents) == 0 {
			t.Errorf("resource %s returned no contents", uri)
			continue
		}
		for _, content := range res.Contents {
			text, ok := content.(mcp.TextResourceContents)
			if !ok {
				t.Errorf("resource %s returned %T, want text contents", uri, content)
				continue
			}
			if text.URI != uri {
				t.Errorf("resource %s labelled its contents %q", uri, text.URI)
			}
		}
	}
}

// TestWorkspaceDiffScope covers the documented `scope` argument. It was declared
// and then dropped, so an agent asking for `stat` to bound its token spend was
// handed the whole patch anyway.
func TestWorkspaceDiffScope(t *testing.T) {
	c, app := setupMCP(t)

	// server.go is committed by gitInit, so editing it gives `git diff HEAD`
	// something to report
	if err := writeFile(filepath.Join(app.Root, "server.go"), "package oauth\n\nfunc Login() {}\n"); err != nil {
		t.Fatal(err)
	}

	full := call(t, c, "workspace.diff", nil)
	if !strings.Contains(full, "func Login() {}") {
		t.Errorf("no scope dropped the patch body: %s", full)
	}

	stat := call(t, c, "workspace.diff", map[string]any{"scope": "stat"})
	if strings.Contains(stat, "func Login() {}") {
		t.Errorf("scope=stat returned the patch body: %s", stat)
	}
	if !strings.Contains(stat, "server.go") || !strings.Contains(stat, "1 file changed") {
		t.Errorf("scope=stat is not a stat summary: %s", stat)
	}

	// an unrecognised scope must widen back to the full diff rather than quietly
	// return less than the caller asked for
	if unknown := call(t, c, "workspace.diff", map[string]any{"scope": "tiny"}); unknown != full {
		t.Errorf("scope=tiny changed the diff:\n got  %s\n want %s", unknown, full)
	}
}

// TestMCPSessionDiffDefaults covers the checkpoint pair session.diff picks when
// the caller omits an ID — the normal call, since both arguments are optional.
// The diff output does not name the checkpoints it compared, so the wrong pair
// reads exactly like an honest empty diff unless the pairing is asserted.
func TestMCPSessionDiffDefaults(t *testing.T) {
	c, app := setupMCP(t)
	ctx := context.Background()

	// init renders the context, and rendering a session with no checkpoint takes
	// one, so a fresh project holds exactly one. Anything else and the call below
	// is no longer the fewer-than-two case.
	cps, err := app.Checkpoint.ListBySession(ctx, appSessionID(app), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cps) != 1 {
		t.Fatalf("a fresh project holds %d checkpoints, this case needs 1", len(cps))
	}

	// One checkpoint has nothing to compare against. Saying so is the only honest
	// answer: diffing it against itself reports "no changes", which an agent reads
	// as "nothing happened".
	if res := callAny(t, c, "session.diff", nil); !strings.Contains(res, "need at least two checkpoints") {
		t.Fatalf("session.diff over a single checkpoint: %s", res)
	}

	first := extractIDLike(t, call(t, c, "session.checkpoint", map[string]any{
		"label": "one", "next_action": "alpha",
	}), "chk_")
	call(t, c, "session.checkpoint", map[string]any{"label": "two", "next_action": "beta"})
	call(t, c, "session.checkpoint", map[string]any{"label": "three", "next_action": "gamma"})

	res := call(t, c, "session.diff", nil)
	if !strings.Contains(res, `"next_action_from":"beta"`) || !strings.Contains(res, `"next_action_to":"gamma"`) {
		t.Errorf("session.diff with no arguments did not compare the two latest checkpoints: %s", res)
	}

	// One side supplied, the other still defaults — the same branch, entered with
	// only one ID missing.
	res = call(t, c, "session.diff", map[string]any{"before_id": first})
	if !strings.Contains(res, `"next_action_from":"alpha"`) || !strings.Contains(res, `"next_action_to":"gamma"`) {
		t.Errorf("session.diff with before_id only did not default after_id to the latest checkpoint: %s", res)
	}
}

// TestMCPContextUpdateFields covers every field context.update accepts. Each one
// writes somewhere different — the current task, a fresh checkpoint, the session
// row — so a field wired to the wrong writer, or quietly dropped from the switch,
// changes nothing while the tool still answers with the record it did not update.
func TestMCPContextUpdateFields(t *testing.T) {
	c, app := setupMCP(t)
	ctx := context.Background()
	sessionID := appSessionID(app)

	taskID := extractID(t, call(t, c, "task.create", map[string]any{"title": "wire the token refresh"}))

	// The task is read back by ID, not through GetCurrent: task_status below
	// completes it, and a completed task is no longer anyone's current task.
	call(t, c, "context.update", map[string]any{"field": "task_title", "value": "rotate refresh tokens"})
	task, err := app.Task.Get(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "rotate refresh tokens" {
		t.Errorf("task_title did not reach the current task, title is %q", task.Title)
	}

	call(t, c, "context.update", map[string]any{"field": "task_status", "value": entities.TaskStatusCompleted})
	task, err = app.Task.Get(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != entities.TaskStatusCompleted {
		t.Errorf("task_status did not reach the current task, status is %q", task.Status)
	}
	if task.Title != "rotate refresh tokens" {
		t.Errorf("task_status overwrote the title with %q", task.Title)
	}

	// next_action is the one field that is not an update: it checkpoints, which is
	// where the next agent looks for what to do next.
	before, err := app.Checkpoint.ListBySession(ctx, sessionID, 100)
	if err != nil {
		t.Fatal(err)
	}
	call(t, c, "context.update", map[string]any{"field": "next_action", "value": "add the rotation test"})
	after, err := app.Checkpoint.ListBySession(ctx, sessionID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("next_action created %d checkpoints, want 1", len(after)-len(before))
	}
	if after[0].NextAction != "add the rotation test" {
		t.Errorf("the checkpoint next_action created carries %q", after[0].NextAction)
	}

	// session_title last: task.create already named the session after its first
	// task, so an unwritten update would otherwise be indistinguishable from that.
	call(t, c, "context.update", map[string]any{"field": "session_title", "value": "OAuth hardening"})
	session, err := app.Session.Get(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Title != "OAuth hardening" {
		t.Errorf("session_title did not reach the session row, title is %q", session.Title)
	}

	// A field the switch does not handle has to fail. Returning success for it
	// would tell an agent its update landed when nothing was written.
	if res := callAny(t, c, "context.update", map[string]any{"field": "task_owner", "value": "codex"}); !strings.Contains(res, `unknown field "task_owner"`) {
		t.Errorf("context.update did not reject a field it cannot write: %s", res)
	}
}

// TestMCPEventAppendAutoCheckpoint covers the one tool whose extra work depends on
// an argument value: a passing test run is checkpointed, everything else is not.
// The checkpoint is the only trace of that branch, so a broken comparison loses
// the green run silently — event.append still answers ok either way.
func TestMCPEventAppendAutoCheckpoint(t *testing.T) {
	// Each case gets its own server: the smart-checkpoint window is per server, so
	// a shared one would suppress the checkpoint the first case has to observe.
	t.Run("test.passed checkpoints", func(t *testing.T) {
		c, app := setupMCP(t)
		sessionID := appSessionID(app)
		before := checkpointCount(t, app, sessionID)

		call(t, c, "event.append", map[string]any{"type": entities.EventTestPassed, "payload": `{"suite":"unit"}`})

		cps, err := app.Checkpoint.ListBySession(context.Background(), sessionID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(cps) != before+1 {
			t.Fatalf("test.passed produced %d checkpoints, want 1", len(cps)-before)
		}
		if cps[0].Label != "tests passed" {
			t.Errorf("the checkpoint test.passed created is labelled %q", cps[0].Label)
		}
	})

	t.Run("other types do not", func(t *testing.T) {
		c, app := setupMCP(t)
		sessionID := appSessionID(app)
		before := checkpointCount(t, app, sessionID)

		call(t, c, "event.append", map[string]any{"type": entities.EventCommandExecuted, "payload": `{"cmd":"go test"}`})

		if after := checkpointCount(t, app, sessionID); after != before {
			t.Errorf("command.executed checkpointed too, so the branch is not on the event type: %d new checkpoints", after-before)
		}
	})
}

func checkpointCount(t *testing.T, app *bootstrap.App, sessionID string) int {
	t.Helper()
	cps, err := app.Checkpoint.ListBySession(context.Background(), sessionID, 100)
	if err != nil {
		t.Fatal(err)
	}
	return len(cps)
}

func resourceURIs(t *testing.T, c *client.Client) []string {
	t.Helper()
	res, err := c.ListResources(context.Background(), mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	uris := make([]string, 0, len(res.Resources))
	for _, r := range res.Resources {
		uris = append(uris, r.URI)
	}
	return uris
}

func readResource(t *testing.T, c *client.Client, uri string) *mcp.ReadResourceResult {
	t.Helper()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri
	res, err := c.ReadResource(context.Background(), req)
	if err != nil {
		t.Fatalf("read resource %s: %v", uri, err)
	}
	return res
}
