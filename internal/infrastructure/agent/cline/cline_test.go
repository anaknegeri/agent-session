package cline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigureRefusesJSONCSettings is the reproduction of the worst clobber this
// repo had: .vscode/settings.json is JSONC — comments and trailing commas are
// legal there and encoding/json rejects both — and setup used to replace the whole
// file with just cline.mcpServers, deleting every workspace setting the user had.
func TestConfigureRefusesJSONCSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vscode", "settings.json")
	const jsonc = "{\n  // my workspace settings\n  \"editor.formatOnSave\": true,\n  \"go.lintTool\": \"golangci-lint\",\n}\n"
	write(t, path, jsonc)

	err := NewAdapter(dir).Configure(context.Background(), "/usr/local/bin/agent-session")
	if err == nil {
		t.Fatal("Configure rewrote a JSONC settings.json")
	}
	if got := read(t, path); got != jsonc {
		t.Errorf("the user's settings were modified:\n%s", got)
	}
	// The reader cannot act on "invalid JSON" alone; the message has to carry the
	// entry they now have to paste in themselves.
	if !strings.Contains(err.Error(), "cline.mcpServers") {
		t.Errorf("the error does not say what to add by hand: %v", err)
	}
}

func TestConfigureKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vscode", "settings.json")
	write(t, path, `{"editor.formatOnSave":true,"cline.mcpServers":{"mine":{"command":"my-mcp"}}}`)

	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}

	config := readJSON(t, path)
	if config["editor.formatOnSave"] != true {
		t.Errorf("an unrelated workspace setting was dropped: %v", config)
	}
	servers, _ := config["cline.mcpServers"].(map[string]any)
	if _, ok := servers["mine"]; !ok {
		t.Error("the user's own cline server was dropped")
	}
	if _, ok := servers["agent-session"]; !ok {
		t.Errorf("agent-session not registered: %v", servers)
	}
	if _, err := os.Stat(filepath.Join(dir, ".clinerules")); err != nil {
		t.Errorf("no rules written: %v", err)
	}

	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	config = readJSON(t, path)
	servers, _ = config["cline.mcpServers"].(map[string]any)
	if _, ok := servers["agent-session"]; ok {
		t.Error("our entry survived uninstall")
	}
	if _, ok := servers["mine"]; !ok {
		t.Error("uninstall took the user's own server with it")
	}
	if config["editor.formatOnSave"] != true {
		t.Error("uninstall dropped an unrelated workspace setting")
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

// TestConfigureWritesIntoClineRulesDirectory is the reproduction of an install
// that produced nothing at all: in a project using Cline's directory form,
// writing `.clinerules` as a file fails with "is a directory", and Configure
// aborted there — before the MCP wiring — so cline got neither rules nor tools.
func TestConfigureWritesIntoClineRulesDirectory(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, ".clinerules")
	const theirs = "# House style\n\nAlways use tabs.\n"
	write(t, filepath.Join(rules, "style.md"), theirs)

	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}

	ours := filepath.Join(rules, "agent-session.md")
	if got := read(t, ours); !strings.Contains(got, "session.checkpoint") {
		t.Errorf("the rule inside .clinerules/ is not the workflow rule:\n%s", got)
	}
	config := readJSON(t, filepath.Join(dir, ".vscode", "settings.json"))
	servers, _ := config["cline.mcpServers"].(map[string]any)
	if _, ok := servers["agent-session"]; !ok {
		t.Errorf("the rules step took the MCP wiring down with it: %v", config)
	}

	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(ours); !os.IsNotExist(err) {
		t.Errorf("our rule survived uninstall and keeps instructing the agent: %v", err)
	}
	if got := read(t, filepath.Join(rules, "style.md")); got != theirs {
		t.Errorf("uninstall touched the project's own rule file:\n%s", got)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

// TestConfigureKeepsHandWrittenClineRules pins the ownership rule: a .clinerules
// the user wrote carries no marker, so it is neither rewritten nor deleted.
func TestConfigureKeepsHandWrittenClineRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".clinerules")
	const mine = "# My rules\n\nNever run the formatter.\n"
	write(t, path, mine)

	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if got := read(t, path); got != mine {
		t.Errorf("the user's .clinerules was rewritten:\n%s", got)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if got := read(t, path); got != mine {
		t.Errorf("uninstall removed or changed the user's .clinerules:\n%s", got)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
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

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return config
}
