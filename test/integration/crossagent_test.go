package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
)

// TestCrossAgentHandoffSmoke is the one thing the per-agent smoke tests cannot
// show: that state one real agent writes is picked up by a different real agent.
// Claude Code records a marker through its MCP server, `handoff codex` renders the
// context, and Codex is then asked what is in progress — in the same session, from
// its own MCP server, in a separate process.
//
// Codex keeps its MCP registration at user scope only, so unlike TestCodexSmoke
// this runs against the developer's real ~/.codex rather than an isolated
// CODEX_HOME: an isolated one would have neither the registration nor the
// credentials. Nothing here writes to it — the test skips if the registration is
// absent instead of installing one.
func TestCrossAgentHandoffSmoke(t *testing.T) {
	requireSmokeAgent(t, "claude")
	requireSmokeAgent(t, "codex")
	bin := requireCodexMCPRegistered(t)
	requireTool(t, bin, "context.read")

	dir := t.TempDir()
	gitInit(t, dir)
	initCLI(t, dir, "--project", "--only", "claude")

	// unique per run, so finding it in Codex's output cannot be a coincidence or a
	// leftover from an earlier session
	marker := fmt.Sprintf("MARKER-%d", time.Now().UnixNano())

	before := readState(t, dir)

	claudeOut := runAgent(t, dir, "claude",
		"claude", "-p",
		"Use the agent-session MCP tools: call task.create with the title "+marker+
			", then call session.record with checkpoint=true and next_action=\"hand over to codex\". Reply DONE.",
		"--dangerously-skip-permissions")

	// Hard assertion on agent A: the marker has to be in the store, whatever the
	// model said in its reply.
	afterClaude := readState(t, dir)
	if afterClaude.currentTask != marker {
		t.Fatalf("claude did not record the task through MCP: current task is %q, want %q\nclaude output: %s",
			afterClaude.currentTask, marker, claudeOut)
	}

	handoff := runCLI(t, dir, "handoff", "codex")
	if !strings.Contains(handoff, marker) {
		t.Errorf("the handoff context does not carry the task the previous agent recorded:\n%s", handoff)
	}

	afterHandoff := readState(t, dir)
	if afterHandoff.sessionID != afterClaude.sessionID {
		t.Errorf("handoff moved the work to a different session: %s -> %s", afterClaude.sessionID, afterHandoff.sessionID)
	}
	if afterHandoff.lastAgent != "codex" {
		t.Errorf("last_agent is %q after handing off to codex", afterHandoff.lastAgent)
	}
	if afterHandoff.checkpoints <= afterClaude.checkpoints {
		t.Errorf("handoff stored no checkpoint: %d -> %d", afterClaude.checkpoints, afterHandoff.checkpoints)
	}

	// context.read, not context.get: `codex exec` runs with approval never, where
	// Codex executes MCP tools annotated read-only and cancels the rest. context.get
	// syncs file changes, so it is honestly not read-only and Codex is right to
	// refuse it — which is exactly why context.read has to exist.
	codexOut := runAgent(t, dir, "codex",
		"codex", "exec", "--skip-git-repo-check", "-s", "read-only",
		"Call the agent-session MCP tools session.get and context.read, then reply with the exact title of the task in progress and nothing else.")

	// The point of the whole test: agent B reports what agent A recorded.
	if !strings.Contains(codexOut, marker) {
		t.Errorf("codex did not report the task claude recorded (%s):\n%s", marker, codexOut)
	}

	final := readState(t, dir)
	if final.sessionID != before.sessionID && before.sessionID != "" {
		t.Errorf("the session changed identity across the handoff: %s -> %s", before.sessionID, final.sessionID)
	}
	if final.events <= afterHandoff.events {
		t.Logf("note: codex's run appended no events of its own (%d), so it read the session without recording to it",
			final.events)
	}
}

// requireCodexMCPRegistered skips unless the developer's Codex config registers
// agent-session, since without it Codex has no way to reach the session and a
// failure would say nothing about cross-agent behaviour. It returns the binary
// Codex is configured to launch — which is the installed one, not the repo build,
// so the rest of the test has to check what that binary can actually do.
func requireCodexMCPRegistered(t *testing.T) string {
	t.Helper()
	dir, err := codex.ConfigDir()
	if err != nil {
		t.Skipf("cannot resolve the codex config dir: %v", err)
	}
	path := filepath.Join(dir, "config.toml")
	cfg, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no codex config at %s; run `agent-session init --only codex`", dir)
	}
	bin := mcpServerCommand(string(cfg))
	if bin == "" {
		t.Skipf("codex has no agent-session MCP server registered in %s; run `agent-session init --only codex`", path)
	}
	return bin
}

// mcpServerCommand pulls the command out of Codex's [mcp_servers.agent-session]
// table. Deliberately a scan rather than a TOML decode: the test only needs one
// value, and reading the developer's real config should not depend on the rest of
// it parsing cleanly.
func mcpServerCommand(cfg string) string {
	_, rest, found := strings.Cut(cfg, "[mcp_servers.agent-session]")
	if !found {
		return ""
	}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			break // next table: the command was not in this one
		}
		if value, ok := strings.CutPrefix(line, "command"); ok {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "=")), `"`)
		}
	}
	return ""
}

// requireTool skips unless the MCP server Codex will actually launch exposes name.
// The registered binary is whatever is installed on PATH, so a tool added in the
// working tree is not there until it ships — without this the test would fail and
// blame the agent for a capability the server never advertised.
func requireTool(t *testing.T, bin, name string) {
	t.Helper()
	c, err := client.NewStdioMCPClient(bin, nil, "mcp")
	if err != nil {
		t.Skipf("cannot start %s as an MCP server: %v", bin, err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Skipf("cannot initialize %s over stdio: %v", bin, err)
	}
	res, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Skipf("cannot list tools of %s: %v", bin, err)
	}
	for _, tool := range res.Tools {
		if tool.Name == name {
			return
		}
	}
	t.Skipf("the agent-session registered with codex (%s) does not expose %s yet; install a build that does", bin, name)
}

// state is the session state a cross-agent run is expected to carry over.
type state struct {
	sessionID   string
	lastAgent   string
	currentTask string
	events      int
	checkpoints int
}

func readState(t *testing.T, dir string) state {
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
	s := state{sessionID: session.ID, lastAgent: session.LastAgent}
	if task, err := app.Task.GetCurrent(ctx, session.ID); err == nil && task != nil {
		s.currentTask = task.Title
	}
	if evs, err := app.Event.List(ctx, session.ID, 1000); err == nil {
		s.events = len(evs)
	}
	if cps, err := app.Checkpoint.ListBySession(ctx, session.ID, 1000); err == nil {
		s.checkpoints = len(cps)
	}
	return s
}

// runAgent runs a real agent CLI in dir with the freshly built binary on PATH,
// skipping (not failing) when the CLI itself cannot run — auth, network and
// upstream capacity are not what this test is about.
func runAgent(t *testing.T, dir, label string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("%s output: %s", label, out)
		t.Skipf("%s run failed (may need auth/network): %v", label, err)
	}
	return string(out)
}

// runCLI runs the repo's agent-session binary in dir and returns its output.
func runCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir(t), "agent-session"), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent-session %v: %v: %s", args, err, out)
	}
	return string(out)
}
