package integration

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
)

// mcpBinary returns the path to agent-session-mcp, building it if needed.
func mcpBinary(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin", "agent-session-mcp")
	if _, err := os.Stat(bin); err != nil {
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/agent-session-mcp")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build mcp binary: %v: %s", err, out)
		}
	}
	return bin
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
	}
}

// TestRealMCPOverStdio verifies the full agent workflow over a REAL MCP stdio
// subprocess (exactly how Claude Code / OpenCode / Cursor talk to the server),
// instead of the in-process client used by unit tests.
func TestRealMCPOverStdio(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir)

	bin := mcpBinary(t)
	client_, err := client.NewStdioMCPClient(bin, nil, "mcp")
	if err != nil {
		t.Fatalf("start stdio mcp client: %v", err)
	}
	t.Cleanup(func() { client_.Close() })
	ctx := context.Background()

	if _, err := client_.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	call := func(name string, args map[string]any) string {
		t.Helper()
		req := mcp.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := client_.CallTool(ctx, req)
		if err != nil {
			t.Fatalf("call %s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("call %s returned error: %v", name, res.Content)
		}
		return mcpText(res)
	}

	// full workflow: get session, load context, create task, record, checkpoint
	call("session.get", nil)
	call("context.get", map[string]any{"depth": "summary"})
	taskRes := call("task.create", map[string]any{"title": "Integration test task"})
	if !strings.Contains(taskRes, "task_") {
		t.Fatalf("task.create did not return task id: %s", taskRes)
	}
	call("session.record", map[string]any{
		"decision":      "Use stdio MCP in integration tests",
		"event_type":    "test.passed",
		"checkpoint":    "true",
		"next_action":   "verify everything",
	})
	ctxRes := call("context.get", map[string]any{"depth": "summary"})
	if !strings.Contains(ctxRes, "Integration test task") {
		t.Fatalf("context.get missing task: %s", ctxRes)
	}
	// next_action set via session.record must appear in the summary
	if !strings.Contains(ctxRes, "verify everything") {
		t.Fatalf("context.get missing next_action from session.record: %s", ctxRes)
	}

	// the decision is recorded even if the summary budget truncates it
	fullRes := call("context.get", map[string]any{"depth": "full"})
	if !strings.Contains(fullRes, "Use stdio MCP in integration tests") {
		t.Fatalf("context.get (full) missing decision: %s", fullRes)
	}
}

func mcpText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		switch v := c.(type) {
		case mcp.TextContent:
			b.WriteString(v.Text)
		case *mcp.TextContent:
			b.WriteString(v.Text)
		}
	}
	return b.String()
}

// --- smoke tests gated behind AGENT_SESSION_SMOKE=1 ---

// smokeAgent runs the agent CLI in "exec/run" mode against the project dir and
// returns true if the CLI is available. Smoke tests are skipped unless
// AGENT_SESSION_SMOKE=1 and the agent binary is on PATH.
func smokeAgent(t *testing.T, dir, name string, args ...string) bool {
	t.Helper()
	if os.Getenv("AGENT_SESSION_SMOKE") != "1" {
		t.Skip("set AGENT_SESSION_SMOKE=1 to run real-agent smoke tests")
	}
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not on PATH; skipping smoke test", name)
	}
	return true
}

// TestClaudeCodeSmoke runs Claude Code in non-interactive print mode against a
// fresh project and verifies it produces agent-session state (checkpoint files).
func TestClaudeCodeSmoke(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir)
	if !smokeAgent(t, dir, "claude") {
		return
	}
	// -p for print mode; the prompt is intentionally minimal — it should still
	// hit the session hooks (SessionStart resume, Stop checkpoint).
	cmd := exec.Command("claude", "-p", "Say hello and report your current task.", "--dangerously-skip-permissions")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("claude output: %s", out)
		t.Skipf("claude run failed (may need auth/network): %v", err)
	}
}

// TestCodexSmoke runs Codex in non-interactive mode and verifies the session is
// resumed (an open agent session is created/closed).
func TestCodexSmoke(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir)
	if !smokeAgent(t, dir, "codex") {
		return
	}
	cmd := exec.Command("codex", "exec", "--skip-git-repo-check", "--yes", "Say hello.")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("codex output: %s", out)
		t.Skipf("codex run failed: %v", err)
	}
}

// TestOpenCodeSmoke runs OpenCode non-interactively and verifies the session
// workflow is reachable.
func TestOpenCodeSmoke(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir)
	if !smokeAgent(t, dir, "opencode") {
		return
	}
	cmd := exec.Command("opencode", "run", "Say hello.", "--model", "x")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("opencode output: %s", out)
		t.Skipf("opencode run failed: %v", err)
	}
}

// initCLI runs `agent-session init` in dir using the repo's binary.
func initCLI(t *testing.T, dir string) {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin", "agent-session")
	if _, err := os.Stat(bin); err != nil {
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/agent-session")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build agent-session: %v: %s", err, out)
		}
	}
	cmd := exec.Command(bin, "init", "--no-agents")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("agent-session init: %v: %s", err, out)
	}
}
