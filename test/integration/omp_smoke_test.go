package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOmpSmoke runs the real omp CLI against a project wired with
// `init --project --only omp`.
//
// omp has both halves of the integration, unlike pi: an MCP client and the pi
// extension API. This test covers the half a model cannot skip — the lifecycle
// extension — because it runs without a provider. The API key below is
// deliberately invalid: omp fires session_start and before_agent_start before it
// contacts a provider and session_shutdown after, so every hook this test cares
// about runs, the model never does, nothing can be spent, and the model cannot
// touch the repository.
func TestOmpSmoke(t *testing.T) {
	requireSmokeAgent(t, "omp")

	dir := t.TempDir()
	gitInit(t, dir)
	// --project keeps the wiring inside the temp dir; the default user scope
	// would write into the developer's own ~/.omp.
	initCLI(t, dir, "--project", "--only", "omp")

	assertProjectOmpWiring(t, dir)

	sessionDir := t.TempDir()
	cmd := exec.Command("omp",
		"-p", "Report your current task.",
		"--session-dir", sessionDir,
		"--provider", "anthropic",
		"--model", "claude-sonnet-4-5",
		"--api-key", "agent-session-smoke-test-not-a-real-key",
		"--max-time", "60", // a provider that retries must not hang the suite
	)
	cmd.Dir = dir
	// A temp HOME so omp reads and writes none of the developer's real ~/.omp.
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	// The run is expected to fail at the provider call. That is the point: the
	// lifecycle hooks either ran before it or they did not, and the assertions
	// below say which.
	out, _ := cmd.CombinedOutput()

	if agent := latestSessionAgent(t, dir); agent != "omp" {
		t.Errorf("session_start did not run `agent-session resume --agent omp`: last agent is %q\nomp output: %s", agent, out)
	}

	labels := checkpointLabels(t, dir)
	shutdownFired := false
	for _, l := range labels {
		if l == "auto" {
			shutdownFired = true
			break
		}
	}
	if !shutdownFired {
		t.Errorf("session_shutdown did not checkpoint: no label %q among %q\nomp output: %s", "auto", labels, out)
	}

	// before_agent_start injects the rendered context as a persistent session
	// entry, which is the only half of the integration that reaches the model.
	// omp stores it in the session file, so it is assertable without a model run.
	if !sessionContainsInjectedContext(t, sessionDir) {
		t.Errorf("before_agent_start injected no agent-session context into the omp session\nomp output: %s", out)
	}
}

// assertProjectOmpWiring checks what `init --project --only omp` writes. omp
// discovers project resources under .omp/ (user scope is ~/.omp/agent/), and a
// file in the wrong one of those two is a file omp never reads.
func assertProjectOmpWiring(t *testing.T, dir string) {
	t.Helper()
	for _, rel := range []string{
		".omp/mcp.json",
		".omp/extensions/agent-session.ts",
		".omp/skills/agent-session/SKILL.md",
		".omp/commands/agent-session.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("init --project --only omp did not write %s: %v", rel, err)
		}
	}

	ext, err := os.ReadFile(filepath.Join(dir, ".omp", "extensions", "agent-session.ts"))
	if err != nil {
		t.Fatalf("read extension: %v", err)
	}
	// An absolute path, not a bare command: omp is often launched from a GUI whose
	// PATH has neither nvm nor Homebrew on it.
	if !strings.Contains(string(ext), `?? "/`) {
		t.Errorf("the extension does not carry an absolute agent-session path:\n%s", ext)
	}
	// The registration form, not the bare event name: extension.ts's header comment
	// names every event, so a bare-name check passes with all handlers deleted.
	for _, hook := range []string{"session_start", "before_agent_start", "session_before_compact", "session_stop", "session_shutdown"} {
		if !strings.Contains(string(ext), `pi.on("`+hook+`"`) {
			t.Errorf("the extension does not register %s:\n%s", hook, ext)
		}
	}

	// The MCP half: omp is the pi-family agent that has a client, so a missing
	// stdio entry means the model has no session tools at all.
	data, err := os.ReadFile(filepath.Join(dir, ".omp", "mcp.json"))
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	var config struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse .omp/mcp.json: %v", err)
	}
	server, ok := config.MCPServers["agent-session"]
	if !ok {
		t.Fatalf(".omp/mcp.json has no agent-session server:\n%s", data)
	}
	if server.Type != "stdio" || len(server.Args) != 1 || server.Args[0] != "mcp" {
		t.Errorf("agent-session is not registered as a stdio `agent-session mcp` server: %+v", server)
	}
	if !filepath.IsAbs(server.Command) {
		t.Errorf("mcp.json command %q is not absolute; a GUI-launched omp would not find it", server.Command)
	}
	if server.Env["AGENT_SESSION_AGENT"] != "omp" {
		t.Errorf("mcp.json does not attribute writes to omp: %v", server.Env)
	}
}
