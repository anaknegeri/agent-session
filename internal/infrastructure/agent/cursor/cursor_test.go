package cursor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Cursor's mcp.json is user-authored: a single typo used to be enough for setup to
// reset it to an empty config and write that back, deleting every other server the
// user had registered.
func TestConfigureRefusesUnparseableMCP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "mcp.json")
	const broken = `{"mcpServers":{"mine":{"command":"my-mcp"},}}`
	write(t, path, broken)

	if err := NewAdapter(dir).Configure(context.Background(), "agent-session"); err == nil {
		t.Fatal("Configure overwrote a mcp.json it could not parse")
	}
	if got := read(t, path); got != broken {
		t.Errorf("the unparseable file was modified:\n%s", got)
	}
}

func TestConfigureAndUninstallKeepOtherServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "mcp.json")
	write(t, path, `{"mcpServers":{"mine":{"command":"my-mcp"}}}`)

	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "/usr/local/bin/agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}

	servers := mcpServers(t, path)
	if _, ok := servers["mine"]; !ok {
		t.Error("registering agent-session dropped the user's own server")
	}
	ours, _ := servers["agent-session"].(map[string]any)
	if ours["command"] != "/usr/local/bin/agent-session" {
		t.Errorf("agent-session not registered: %v", servers)
	}
	// The rule file is what makes Cursor follow the workflow at all.
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "rules", "agent-session.mdc")); err != nil {
		t.Errorf("no rule written: %v", err)
	}

	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	servers = mcpServers(t, path)
	if _, ok := servers["agent-session"]; ok {
		t.Error("our entry survived uninstall")
	}
	if _, ok := servers["mine"]; !ok {
		t.Error("uninstall took the user's own server with it")
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

func TestUninstallRemovesAFileItCreated(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Error("mcp.json survived uninstall with nothing of the user's in it")
	}
}

func mcpServers(t *testing.T, path string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	servers, _ := config["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatalf("%s has no mcpServers map: %v", path, config)
	}
	return servers
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
