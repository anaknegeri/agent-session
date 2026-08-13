package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// agent.instructions.system is the only place OpenCode keeps a user's always-on
// prompt. The project adapter used to assign it, so setup silently replaced
// whatever the user had written there.
func TestConfigureAppendsInstructions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	const mine = "Always answer in Indonesian."
	write(t, path, `{"agent":{"instructions":{"system":"`+mine+`"}},"mcp":{"mine":{"type":"local"}}}`)

	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "/usr/local/bin/agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}

	system := systemInstruction(t, path)
	if !strings.HasPrefix(system, mine) {
		t.Errorf("the user's own instruction was replaced:\n%s", system)
	}
	if !strings.Contains(system, "Agent Session") {
		t.Errorf("the Agent Session note was not appended:\n%s", system)
	}
	servers := section(t, path, "mcp")
	if _, ok := servers["mine"]; !ok {
		t.Error("the user's own MCP server was dropped")
	}
	ours, _ := servers["agent-session"].(map[string]any)
	command, _ := ours["command"].([]any)
	if len(command) != 2 || command[0] != "/usr/local/bin/agent-session" || command[1] != "mcp" {
		t.Errorf("agent-session not registered as a local command: %v", ours)
	}

	// Idempotent: a second run must not append the note twice.
	if err := a.Configure(ctx, "/usr/local/bin/agent-session"); err != nil {
		t.Fatalf("second configure: %v", err)
	}
	if got := strings.Count(systemInstruction(t, path), "Agent Session. At the start"); got != 1 {
		t.Errorf("the note appears %d times after a re-run", got)
	}

	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if got := systemInstruction(t, path); got != mine {
		t.Errorf("uninstall did not restore the user's instruction exactly:\n%q", got)
	}
	servers = section(t, path, "mcp")
	if _, ok := servers["agent-session"]; ok {
		t.Error("our MCP entry survived uninstall")
	}
	if _, ok := servers["mine"]; !ok {
		t.Error("uninstall took the user's own MCP server with it")
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

func TestConfigureRefusesUnparseableConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	const broken = `{"mcp":{"mine":{"type":"local"},}}`
	write(t, path, broken)

	if err := NewAdapter(dir).Configure(context.Background(), "agent-session"); err == nil {
		t.Fatal("Configure overwrote an opencode.json it could not parse")
	}
	if got := read(t, path); got != broken {
		t.Errorf("the unparseable file was modified:\n%s", got)
	}
}

// A project standardized on opencode.jsonc got a second opencode.json it does not
// maintain, so the wiring lived in a file OpenCode may never have been told about.
func TestConfigureWritesTheFileTheProjectUses(t *testing.T) {
	dir := t.TempDir()
	jsonc := filepath.Join(dir, "opencode.jsonc")
	write(t, jsonc, `{"mcp":{}}`)

	if err := NewAdapter(dir).Configure(context.Background(), "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); !os.IsNotExist(err) {
		t.Error("a second opencode.json was created beside the project's opencode.jsonc")
	}
	if _, ok := section(t, jsonc, "mcp")["agent-session"]; !ok {
		t.Error("agent-session was not registered in opencode.jsonc")
	}
}

func systemInstruction(t *testing.T, path string) string {
	t.Helper()
	agentCfg := section(t, path, "agent")
	instructions, _ := agentCfg["instructions"].(map[string]any)
	system, _ := instructions["system"].(string)
	return system
}

func section(t *testing.T, path, key string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	s, _ := config[key].(map[string]any)
	if s == nil {
		t.Fatalf("%s has no %q section: %v", path, key, config)
	}
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
