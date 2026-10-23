package cursor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
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

// The rule used to be written only when the file was absent or empty, so a
// corrected rule could never reach anyone who had already run init — and the rule
// is the only thing that makes Cursor follow the workflow at all.
func TestConfigureUpdatesItsOwnRule(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}

	// A rule shipped by an older release: our marker, no digest, an out-of-date
	// body. It is still ours, so a corrected rule has to reach it.
	stale := stripStamp(strings.Replace(read(t, RulesPath(dir)), "session.get", "session.obsolete", 1))
	if !strings.Contains(stale, "session.obsolete") {
		t.Fatal("test setup did not change the rule body")
	}
	write(t, RulesPath(dir), stale)

	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("re-configure: %v", err)
	}
	if got := read(t, RulesPath(dir)); strings.Contains(got, "session.obsolete") {
		t.Errorf("a stale rule of ours was never corrected:\n%s", got)
	}
}

// Cursor parses `---` only as the very first line, so the marker has to come
// after the frontmatter.
func TestRuleKeepsFrontmatterFirst(t *testing.T) {
	dir := t.TempDir()
	if err := NewAdapter(dir).Configure(context.Background(), "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	got := read(t, RulesPath(dir))
	if !strings.HasPrefix(got, "---\ndescription:") {
		t.Fatalf("rule does not start with frontmatter:\n%s", got)
	}
	if !strings.Contains(got, agent.ManagedMarker) {
		t.Fatalf("rule carries no ownership marker, so it can never be updated:\n%s", got)
	}
}

func TestConfigureLeavesAUserWrittenRuleAlone(t *testing.T) {
	dir := t.TempDir()
	const mine = "---\ndescription: my own cursor rule\n---\n\nDo it my way.\n"
	write(t, RulesPath(dir), mine)

	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if got := read(t, RulesPath(dir)); got != mine {
		t.Errorf("a hand-written cursor rule was overwritten:\n%s", got)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if got := read(t, RulesPath(dir)); got != mine {
		t.Errorf("uninstall deleted a rule it never wrote:\n%s", got)
	}
}

// Leaving the rule behind kept Cursor injecting alwaysApply instructions to call
// MCP tools the same uninstall had just deregistered.
func TestUninstallRemovesTheRule(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter(dir)
	ctx := context.Background()
	if err := a.Configure(ctx, "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(RulesPath(dir)); !os.IsNotExist(err) {
		t.Errorf("the alwaysApply rule survived uninstall: %v", err)
	}
	if err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

// stripStamp turns a written file back into renderer output, so a test can hand
// it to WriteManaged and get a correctly stamped file for a modified body.
func stripStamp(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !strings.Contains(line, agent.ManagedMarker) {
			continue
		}
		fields := strings.Fields(line)
		kept := fields[:0]
		for _, f := range fields {
			if strings.HasPrefix(f, "version=") || strings.HasPrefix(f, "sha256=") {
				continue
			}
			kept = append(kept, f)
		}
		lines[i] = strings.Join(kept, " ")
		break
	}
	return strings.Join(lines, "\n")
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
