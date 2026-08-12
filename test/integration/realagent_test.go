package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
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
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/agent-session-mcp")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcp binary: %v: %s", err, out)
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
	initCLI(t, dir, "--no-agents")

	bin := mcpBinary(t)
	// start the MCP server with the temp project as its working directory, so
	// it resolves that project's .agent/ (CI runs from a different cwd)
	client_, err := client.NewStdioMCPClientWithOptions(bin, nil, []string{"mcp"},
		transport.WithCommandFunc(func(ctx context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
			cmd := exec.CommandContext(ctx, command, args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), env...)
			return cmd, nil
		}),
	)
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
		"decision":    "Use stdio MCP in integration tests",
		"event_type":  "test.passed",
		"checkpoint":  true,
		"next_action": "verify everything",
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

// requireSmokeAgent skips the calling test unless smoke tests are enabled and
// the agent CLI is on PATH. It never returns when it skips.
func requireSmokeAgent(t *testing.T, name string) {
	t.Helper()
	if os.Getenv("AGENT_SESSION_SMOKE") != "1" {
		t.Skip("set AGENT_SESSION_SMOKE=1 to run real-agent smoke tests")
	}
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not on PATH; skipping smoke test", name)
	}
}

// sessionCounts reports the session state an agent run is expected to move:
// appended events and stored checkpoints for the project's latest session.
func sessionCounts(t *testing.T, dir string) (events, checkpoints int) {
	t.Helper()
	app, err := bootstrap.Open(dir)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer app.Store.Close()

	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, dir)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.GetLatest(ctx, projectID)
	if err != nil {
		t.Fatalf("get latest session: %v", err)
	}
	evs, err := app.Event.List(ctx, session.ID, 1000)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	cps, err := app.Checkpoint.ListBySession(ctx, session.ID, 1000)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	return len(evs), len(cps)
}

// checkpointLabels returns the labels of every checkpoint in the project's latest
// session. The installed hooks use fixed labels (`auto` for Stop, `precompact`
// for PreCompact), so a label is attributable to a hook rather than to the agent
// happening to call an MCP tool.
func checkpointLabels(t *testing.T, dir string) []string {
	t.Helper()
	app, err := bootstrap.Open(dir)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer app.Store.Close()

	ctx := context.Background()
	projectID, err := app.ResolveProjectID(ctx, dir)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	session, err := app.Session.GetLatest(ctx, projectID)
	if err != nil {
		t.Fatalf("get latest session: %v", err)
	}
	cps, err := app.Checkpoint.ListBySession(ctx, session.ID, 1000)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	labels := make([]string, 0, len(cps))
	for _, cp := range cps {
		labels = append(labels, cp.Label)
	}
	return labels
}

// TestClaudeCodeSmoke runs Claude Code in print mode against a freshly wired
// project. Claude is the one agent with real hooks (SessionStart resume, Stop
// checkpoint), so the run must move session state.
//
// Attribution caveat: Claude merges user-scope and project-scope settings, so on
// a machine where the developer has already run `agent-session init` at user
// scope, the same hooks fire regardless of what this test installs. The test
// therefore checks the project wiring as files (which is scope-attributable) and
// reports when the behavioural half cannot be attributed to it.
func TestClaudeCodeSmoke(t *testing.T) {
	requireSmokeAgent(t, "claude")

	dir := t.TempDir()
	gitInit(t, dir)
	// --project keeps .mcp.json/.claude/hooks inside the temp dir; the default
	// user scope would rewrite the developer's own ~/.claude configuration.
	initCLI(t, dir, "--project", "--only", "claude")

	assertProjectClaudeWiring(t, dir)

	beforeEvents, beforeCheckpoints := sessionCounts(t, dir)

	cmd := exec.Command("claude", "-p", "Report your current task.", "--dangerously-skip-permissions")
	cmd.Dir = dir
	// the installed hooks invoke a bare `agent-session`, so the freshly built
	// binary has to be on PATH or the hooks fail and the assertions below would
	// blame the product for an environment problem
	cmd.Env = append(os.Environ(), "PATH="+binDir(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("claude output: %s", out)
		t.Skipf("claude run failed (may need auth/network): %v", err)
	}

	afterEvents, afterCheckpoints := sessionCounts(t, dir)
	if afterEvents <= beforeEvents {
		t.Errorf("the run appended no session events: %d before, %d after\nclaude output: %s",
			beforeEvents, afterEvents, out)
	}
	if afterCheckpoints <= beforeCheckpoints {
		t.Errorf("the run created no checkpoint: %d before, %d after\nclaude output: %s",
			beforeCheckpoints, afterCheckpoints, out)
	}

	// Claude gets both hooks and the MCP server, so a bare count could move
	// because the agent called a tool. The Stop hook's fixed `auto` label is what
	// distinguishes a hook firing from ordinary agent activity.
	labels := checkpointLabels(t, dir)
	stopHookFired := false
	for _, l := range labels {
		if l == "auto" {
			stopHookFired = true
			break
		}
	}
	if !stopHookFired {
		t.Errorf("Stop hook did not run: no checkpoint labelled \"auto\" among %q\nclaude output: %s",
			labels, out)
	} else if userScopeHooksInstalled(t) {
		t.Logf("note: ~/.claude/settings.json also wires agent-session, so this run's " +
			"hooks cannot be attributed to the project-scope wiring. Run on a machine " +
			"without user-scope wiring (or in CI) to test that path.")
	}
}

// assertProjectClaudeWiring checks what `init --project --only claude` is
// supposed to write. Unlike the behavioural assertions, this is attributable to
// project scope: the files either exist under dir or they do not.
func assertProjectClaudeWiring(t *testing.T, dir string) {
	t.Helper()
	for _, rel := range []string{".mcp.json", ".claude/settings.json", ".claude/CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("init --project --only claude did not write %s: %v", rel, err)
		}
	}
	settings, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read project settings: %v", err)
	}
	for _, hook := range []string{"SessionStart", "Stop", "PreCompact"} {
		if !strings.Contains(string(settings), hook) {
			t.Errorf("project settings.json is missing the %s hook:\n%s", hook, settings)
		}
	}
	if !strings.Contains(string(settings), "agent-session") {
		t.Errorf("project settings.json hooks do not invoke agent-session:\n%s", settings)
	}
}

// userScopeHooksInstalled reports whether the developer's own Claude settings
// already wire agent-session, which makes hook behaviour in a temp project
// unattributable to that project's wiring.
func userScopeHooksInstalled(t *testing.T) bool {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "agent-session")
}

// TestOpenCodeSmoke runs OpenCode non-interactively against a wired project.
// OpenCode has no hooks — it reaches agent-session through MCP tools and
// instructions only — so state movement is up to the model and cannot be
// asserted. What this proves is that the generated opencode.json is valid
// enough for the CLI to start and that the session survives the run.
func TestOpenCodeSmoke(t *testing.T) {
	requireSmokeAgent(t, "opencode")

	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir, "--project", "--only", "opencode")

	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); err != nil {
		t.Fatalf("init did not write opencode.json: %v", err)
	}

	cmd := exec.Command("opencode", "run", "Say hello.")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("opencode output: %s", out)
		t.Skipf("opencode run failed: %v", err)
	}

	// the project must still be readable after the agent touched it
	sessionCounts(t, dir)
}

// TestCodexSmoke covers Codex in two parts.
//
// Codex keeps its MCP registration in a user-level config, but it honours
// CODEX_HOME — so pointing that at a temp directory makes the wiring both
// isolated from the developer's real ~/.codex and attributable to this test.
// The behavioural half then runs the real CLI (which needs the real CODEX_HOME
// for credentials) and checks the session store survives it.
func TestCodexSmoke(t *testing.T) {
	requireSmokeAgent(t, "codex")

	dir := t.TempDir()
	gitInit(t, dir)

	codexHome := t.TempDir()
	initCLIEnv(t, dir, []string{"CODEX_HOME=" + codexHome}, "--only", "codex")

	cfg, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("init --only codex wrote no config under CODEX_HOME: %v", err)
	}
	if !strings.Contains(string(cfg), "[mcp_servers.agent-session]") {
		t.Errorf("codex config is missing the agent-session MCP server:\n%s", cfg)
	}

	beforeEvents, _ := sessionCounts(t, dir)

	// read-only sandbox: the smoke test must not let the model modify the repo
	cmd := exec.Command("codex", "exec", "--skip-git-repo-check", "-s", "read-only", "Say hello.")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("codex output: %s", out)
		t.Skipf("codex run failed (may need auth/network): %v", err)
	}

	afterEvents, _ := sessionCounts(t, dir)
	if afterEvents < beforeEvents {
		t.Errorf("session events went backwards after codex run: %d before, %d after\ncodex output: %s",
			beforeEvents, afterEvents, out)
	}
}

// binDir returns the repo's bin directory, where the test binaries are built.
func binDir(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "bin")
}

// initCLI runs `agent-session init` in dir using the repo's binary. Pass the
// wiring flags the test needs; use --no-agents for none.
func initCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	initCLIEnv(t, dir, nil, args...)
}

// initCLIEnv is initCLI with extra environment, for wiring that a test needs to
// redirect away from the developer's own config (for example CODEX_HOME).
func initCLIEnv(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin", "agent-session")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/agent-session")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent-session: %v: %s", err, out)
	}
	run := exec.Command(bin, append([]string{"init"}, args...)...)
	run.Dir = dir
	run.Env = append(os.Environ(), env...)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("agent-session init %v: %v: %s", args, err, out)
	}
}
