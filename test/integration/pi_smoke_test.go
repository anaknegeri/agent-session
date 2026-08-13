package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

// TestPiSmoke runs the real pi CLI against a project wired with
// `init --project --only pi`.
//
// pi has no MCP client, so nothing here goes through MCP: the whole integration
// is the extension calling the agent-session CLI. That makes the run cheap to
// assert and, unlike the other smoke tests, free of credentials — the API key
// below is deliberately invalid. pi fires session_start and before_agent_start
// *before* it contacts a provider and session_shutdown after, so every hook this
// test cares about runs and the model never does. Nothing can be spent, and the
// model cannot touch the repository.
func TestPiSmoke(t *testing.T) {
	requireSmokeAgent(t, "pi")

	dir := t.TempDir()
	gitInit(t, dir)
	// --project keeps the wiring inside the temp dir; the default user scope
	// would write into the developer's own ~/.pi.
	initCLI(t, dir, "--project", "--only", "pi")

	assertProjectPiWiring(t, dir)

	sessionDir := t.TempDir()
	// A temp HOME so pi reads and writes none of the developer's real ~/.pi —
	// including its trust store, which this test must not modify.
	cmd := exec.Command("pi",
		"-p", "Report your current task.",
		"--approve", // trust project-local .pi for this run only; nothing persisted
		"--session-dir", sessionDir,
		"--provider", "anthropic",
		"--model", "claude-sonnet-4-5",
		"--api-key", "agent-session-smoke-test-not-a-real-key",
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"PI_OFFLINE=1",
	)
	// The run is expected to fail at the provider call. That is the point: the
	// lifecycle hooks either ran before it or they did not, and the assertions
	// below say which.
	out, _ := cmd.CombinedOutput()

	if agent := latestSessionAgent(t, dir); agent != "pi" {
		t.Errorf("session_start did not run `agent-session resume --agent pi`: last agent is %q\npi output: %s", agent, out)
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
		t.Errorf("session_shutdown did not checkpoint: no label %q among %q\npi output: %s", "auto", labels, out)
	}

	// before_agent_start injects the rendered context as a persistent session
	// entry, which is the only half of the integration that reaches the model.
	// pi stores it in the session file, so it is assertable without a model run.
	if !sessionContainsInjectedContext(t, sessionDir) {
		t.Errorf("before_agent_start injected no agent-session context into the pi session\npi output: %s", out)
	}
}

// assertProjectPiWiring checks what `init --project --only pi` writes. pi
// discovers project resources under .pi/ (user scope is ~/.pi/agent/), and a
// file in the wrong one of those two is a file pi never reads.
func assertProjectPiWiring(t *testing.T, dir string) {
	t.Helper()
	for _, rel := range []string{
		".pi/extensions/agent-session.ts",
		".pi/skills/agent-session/SKILL.md",
		".pi/prompts/agent-session.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("init --project --only pi did not write %s: %v", rel, err)
		}
	}
	ext, err := os.ReadFile(filepath.Join(dir, ".pi", "extensions", "agent-session.ts"))
	if err != nil {
		t.Fatalf("read extension: %v", err)
	}
	// An absolute path, not a bare command: pi is often launched from a GUI whose
	// PATH has neither nvm nor Homebrew on it.
	if !strings.Contains(string(ext), `?? "/`) {
		t.Errorf("the extension does not carry an absolute agent-session path:\n%s", ext)
	}
	for _, hook := range []string{"session_start", "before_agent_start", "session_before_compact", "session_shutdown"} {
		if !strings.Contains(string(ext), hook) {
			t.Errorf("the extension does not handle %s:\n%s", hook, ext)
		}
	}
}

// latestSessionAgent returns the last agent recorded on the project's latest
// session. `resume --agent pi` is what sets it, so it attributes the movement to
// the extension rather than to anything the model did.
func latestSessionAgent(t *testing.T, dir string) string {
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
	return session.LastAgent
}

// sessionContainsInjectedContext reports whether any session file the agent wrote
// carries the custom entry the extension injects. pi and omp share the entry
// shape because they share the extension API.
func sessionContainsInjectedContext(t *testing.T, sessionDir string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(sessionDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), `"customType":"agent-session"`) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk session dir: %v", err)
	}
	return found
}
