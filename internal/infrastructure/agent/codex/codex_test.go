package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hooksFile reads $CODEX_HOME/hooks.json as a generic tree.
func hooksFile(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse hooks.json: %v\n%s", err, data)
	}
	return out
}

// commandsFor returns the hook commands registered for an event.
func commandsFor(t *testing.T, tree map[string]any, event string) []string {
	t.Helper()
	hooks, ok := tree["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks.json has no hooks object: %v", tree)
	}
	entries, ok := hooks[event].([]any)
	if !ok {
		return nil
	}
	var cmds []string
	for _, e := range entries {
		group, ok := e.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range inner {
			hook, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hook["command"].(string); ok {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

// TestInstallWritesSessionHooks verifies Codex gets the same automatic
// resume/checkpoint behaviour Claude Code has. Codex supports hooks; agent-session
// simply never wired them, so a Codex session had to rely on the model choosing to
// call the MCP tools.
func TestInstallWritesSessionHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	a := NewAdapter()
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	tree := hooksFile(t, home)
	for event, want := range map[string]string{
		"SessionStart": "resume",
		"Stop":         "checkpoint",
	} {
		cmds := commandsFor(t, tree, event)
		if len(cmds) == 0 {
			t.Errorf("%s has no hook command", event)
			continue
		}
		joined := strings.Join(cmds, " ")
		if !strings.Contains(joined, "agent-session "+want) {
			t.Errorf("%s command = %q, want it to run agent-session %s", joined, joined, want)
		}
		if !strings.Contains(joined, ".agent") {
			t.Errorf("%s command should be guarded on a project having .agent/: %q", event, joined)
		}
	}
}

// TestInstallPreservesForeignHooks verifies wiring merges into an existing
// hooks.json instead of replacing it — the file is shared with plugins.
func TestInstallPreservesForeignHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	existing := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/other/tool.sh"}]}],"SessionStart":[{"hooks":[{"type":"command","command":"/other/start.sh"}]}]}}`
	if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAdapter()
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	tree := hooksFile(t, home)
	if got := strings.Join(commandsFor(t, tree, "UserPromptSubmit"), " "); !strings.Contains(got, "/other/tool.sh") {
		t.Errorf("an unrelated event was dropped: %q", got)
	}
	start := strings.Join(commandsFor(t, tree, "SessionStart"), " ")
	if !strings.Contains(start, "/other/start.sh") {
		t.Errorf("another tool's SessionStart hook was dropped: %q", start)
	}
	if !strings.Contains(start, "agent-session resume") {
		t.Errorf("our SessionStart hook was not added: %q", start)
	}
}

// TestInstallIsIdempotent verifies re-running install does not duplicate hooks.
func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	a := NewAdapter()
	for i := 0; i < 3; i++ {
		if err := a.Install(context.Background()); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
	}
	tree := hooksFile(t, home)
	for _, event := range []string{"SessionStart", "Stop"} {
		cmds := commandsFor(t, tree, event)
		ours := 0
		for _, c := range cmds {
			if strings.Contains(c, "agent-session") {
				ours++
			}
		}
		if ours != 1 {
			t.Errorf("%s has %d agent-session hooks after 3 installs, want 1", event, ours)
		}
	}
}

// TestUninstallRemovesOnlyOurHooks verifies uninstall leaves other tools alone.
func TestUninstallRemovesOnlyOurHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	existing := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/other/start.sh"}]}]}}`
	if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAdapter()
	if err := a.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	tree := hooksFile(t, home)
	start := strings.Join(commandsFor(t, tree, "SessionStart"), " ")
	if strings.Contains(start, "agent-session") {
		t.Errorf("our hook survived uninstall: %q", start)
	}
	if !strings.Contains(start, "/other/start.sh") {
		t.Errorf("uninstall removed another tool's hook: %q", start)
	}
}

// TestUninstallWithNothingInstalledWritesNothing is the reproduction of two
// uninstall defects: uninstalling on a machine where hooks were never installed
// *created* hooks.json holding `{"hooks":{}}`, and a missing $CODEX_HOME made
// uninstall fail outright, so `plugin uninstall codex` could not be re-run.
func TestUninstallWithNothingInstalledWritesNothing(t *testing.T) {
	home := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", home)

	a := NewAdapter()
	for i := range 2 {
		if err := a.Uninstall(context.Background()); err != nil {
			t.Fatalf("uninstall %d: %v", i, err)
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("uninstall created %s", home)
	}
}

// TestUninstallLeavesNoEmptyHooksFile verifies install/uninstall is a round trip:
// a hooks.json that held only our entries is removed rather than left behind
// holding an empty object.
func TestUninstallLeavesNoEmptyHooksFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	a := NewAdapter()
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}
	for i := range 2 {
		if err := a.Uninstall(context.Background()); err != nil {
			t.Fatalf("uninstall %d: %v", i, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(home, "hooks.json")); err == nil {
		t.Errorf("hooks.json was left behind after uninstall: %s", data)
	}
}

// TestUninstallKeepsAForeignOnlyHooksFileByteIdentical verifies we do not rewrite
// (and so reformat) a hooks.json that has nothing of ours in it.
func TestUninstallKeepsAForeignOnlyHooksFileByteIdentical(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	path := filepath.Join(home, "hooks.json")
	const existing = "{\"hooks\":{\"SessionStart\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"/other/start.sh\"}]}]}}"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := NewAdapter().Uninstall(context.Background()); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("uninstall removed another tool's hooks.json: %v", err)
	}
	if string(data) != existing {
		t.Errorf("uninstall rewrote a hooks.json it had no entry in:\n%s", data)
	}
}

// TestUninstallRemovesMCPSubTables is the reproduction of the orphan sub-table:
// [mcp_servers.agent-session.env] starts with `[`, which used to end the skip, so
// uninstall left a table re-declaring the server with no command in it.
func TestUninstallRemovesMCPSubTables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	path := filepath.Join(home, "config.toml")
	const config = `model = "gpt-5"

[mcp_servers.other]
command = "other-mcp"
args = ["mcp"]

[mcp_servers.agent-session]
command = "agent-session"
args = ["mcp"]

[mcp_servers.agent-session.env]
AGENT_SESSION_AGENT = "codex"

[projects."/home/me/app"]
trust_level = "trusted"
`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := NewAdapter().Uninstall(context.Background()); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	const want = `model = "gpt-5"

[mcp_servers.other]
command = "other-mcp"
args = ["mcp"]

[projects."/home/me/app"]
trust_level = "trusted"
`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if string(data) != want {
		t.Errorf("config.toml after uninstall:\n%s\nwant:\n%s", data, want)
	}
}

// TestUninstallRemovesQuotedMCPSection covers the other spelling of the same
// table, and pins the prefix boundary: agent-session-old is someone else's server.
func TestUninstallRemovesQuotedMCPSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	path := filepath.Join(home, "config.toml")
	const config = `[mcp_servers."agent-session"]
command = "agent-session"

[mcp_servers."agent-session".env]
AGENT_SESSION_AGENT = "codex"

[mcp_servers.agent-session-old]
command = "legacy"
`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := NewAdapter().Uninstall(context.Background()); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	const want = `[mcp_servers.agent-session-old]
command = "legacy"
`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if string(data) != want {
		t.Errorf("config.toml after uninstall:\n%s\nwant:\n%s", data, want)
	}
}
