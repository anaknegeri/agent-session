package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProjectWiringPreservesUserFiles is the regression test for the worst thing
// this adapter used to do: `init --project --only claude` rebuilt .mcp.json,
// .claude/settings.json and .claude/CLAUDE.md from scratch, so a project's own
// MCP servers, permissions and memory file were destroyed by setup.
func TestProjectWiringPreservesUserFiles(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	writeFile(t, filepath.Join(dir, ".mcp.json"),
		`{"mcpServers":{"mine":{"command":"my-mcp"}}}`)
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(ls)"]},"model":"opus"}`)
	const memory = "# My project memory\n\nAlways use tabs.\n"
	writeFile(t, a.RulePath(), memory)

	if err := a.Configure(context.Background(), "/usr/local/bin/agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	mcp := readJSON(t, a.MCPPath())
	servers, _ := mcp["mcpServers"].(map[string]any)
	if _, ok := servers["mine"]; !ok {
		t.Error("Configure dropped the project's own MCP server")
	}
	ours, _ := servers["agent-session"].(map[string]any)
	if ours["command"] != "/usr/local/bin/agent-session" {
		t.Errorf("agent-session not registered: %v", servers)
	}

	settings := readJSON(t, a.SettingsPath())
	if _, ok := settings["permissions"]; !ok {
		t.Error("Install dropped the project's permissions")
	}
	if settings["model"] != "opus" {
		t.Error("Install dropped the project's model setting")
	}
	if !HasAgentSessionHooks(settings) {
		t.Errorf("Install wrote no agent-session hooks: %v", settings["hooks"])
	}

	rule := readFile(t, a.RulePath())
	if !strings.HasPrefix(rule, memory) {
		t.Errorf("Install overwrote the project's CLAUDE.md:\n%s", rule)
	}
	if !strings.Contains(rule, ruleSection) {
		t.Errorf("Install appended no Agent Session section:\n%s", rule)
	}
}

// Project hooks have to carry the same guards as the user-scope ones: a hook
// subprocess does not inherit the project's working directory, and agent-session
// resolves the project from os.Getwd(), so an unguarded command can checkpoint the
// wrong project or fail the Stop hook outright.
func TestProjectHooksAreGuarded(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	raw := readFile(t, a.SettingsPath())
	for _, want := range []string{"$CLAUDE_PROJECT_DIR", "|| true", "resume --agent claude", "checkpoint --label auto", "checkpoint --label precompact"} {
		if !strings.Contains(raw, want) {
			t.Errorf("project hooks do not contain %q:\n%s", want, raw)
		}
	}
}

func TestProjectUninstallRemovesOnlyOurs(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers":{"mine":{"command":"my-mcp"}}}`)
	writeFile(t, a.SettingsPath(), `{"permissions":{"allow":["Bash(ls)"]}}`)
	const memory = "# My project memory\n\nAlways use tabs.\n"
	writeFile(t, a.RulePath(), memory)

	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Install(ctx); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	mcp := readJSON(t, a.MCPPath())
	servers, _ := mcp["mcpServers"].(map[string]any)
	if _, ok := servers["agent-session"]; ok {
		t.Error("our MCP entry survived uninstall")
	}
	if _, ok := servers["mine"]; !ok {
		t.Error("uninstall took the project's own MCP server with it")
	}

	settings := readJSON(t, a.SettingsPath())
	if _, ok := settings["permissions"]; !ok {
		t.Error("uninstall deleted the project's permissions")
	}
	if HasAgentSessionHooks(settings) {
		t.Error("our hooks survived uninstall")
	}

	if got := readFile(t, a.RulePath()); got != memory {
		t.Errorf("uninstall did not restore CLAUDE.md to the user's content:\n%q", got)
	}

	// Re-runnable: `plugin uninstall` may be called twice.
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

// Files that exist only because of us are removed, so uninstall does not leave
// `{}` litter in a project that had no Claude config to begin with.
func TestProjectUninstallRemovesFilesItCreated(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Install(ctx); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	for _, path := range []string{a.MCPPath(), a.SettingsPath(), a.RulePath()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall in a project that had no Claude config", path)
		}
	}
}

// A .mcp.json with a typo still holds the user's servers: setup must report it,
// not start over from an empty config.
func TestConfigureRefusesUnparseableMCP(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	const broken = `{"mcpServers":{"mine":{"command":"my-mcp"},}}`
	writeFile(t, a.MCPPath(), broken)

	if err := a.Configure(context.Background(), "agent-session"); err == nil {
		t.Fatal("Configure overwrote a .mcp.json it could not parse")
	}
	if got := readFile(t, a.MCPPath()); got != broken {
		t.Errorf("the unparseable file was modified:\n%s", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return config
}
