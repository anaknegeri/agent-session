package omp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

// Every test here works inside t.TempDir(). Nothing may touch the developer's
// real ~/.omp: an adapter test that rewrites the machine's agent configuration is
// a test that has to be run carefully, which means it stops being run.

func TestEnsureResourcesWritesEverything(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	bin := "/usr/local/bin/agent-session"

	if err := EnsureResources(root, bin); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}

	ext := read(t, ExtensionPath(root))
	if !strings.Contains(ext, `"`+bin+`"`) {
		t.Errorf("the extension does not carry the binary path %q:\n%s", bin, ext)
	}
	if strings.Contains(ext, binPlaceholder) {
		t.Error("the binary placeholder was left unreplaced, so the extension cannot run")
	}
	if !strings.Contains(ext, managedMarker) {
		t.Error("the extension has no managed marker, so uninstall will not remove it")
	}
	// extension.ts started as a copy of pi's, so the regression it invites is a
	// leftover `--agent pi`: every omp session would then be recorded as pi.
	if !strings.Contains(ext, `"resume", "--agent", "omp"`) {
		t.Errorf("the extension does not resume as omp:\n%s", ext)
	}
	if strings.Contains(ext, `"--agent", "pi"`) {
		t.Error("the extension still attributes the session to pi")
	}
	// The registration form, not the bare event name: the header comment names
	// every event, so `Contains(ext, "session_stop")` would pass with all four
	// handlers deleted.
	for _, hook := range []string{
		`pi.on("session_start"`,
		`pi.on("before_agent_start"`,
		`pi.on("session_before_compact"`,
		`pi.on("session_stop"`,
		`pi.on("session_shutdown"`,
	} {
		if !strings.Contains(ext, hook) {
			t.Errorf("the extension does not register %s:\n%s", hook, ext)
		}
	}
	// omp abandons a session_shutdown handler after 2s, so a checkpoint asking for
	// the generic 15s budget there is cut off on any repo where git is slow.
	if !strings.Contains(ext, "SHUTDOWN_TIMEOUT_MS") {
		t.Error("the shutdown checkpoint does not use a budget that fits omp's 2s teardown cap")
	}
	// One resume per process: the same module instance is handed to every task
	// subagent runner, and resume closes and reopens the agent_session row.
	if !strings.Contains(ext, "if (resumed) return;") {
		t.Error("session_start is not guarded, so every subagent session would resume again")
	}

	skill := read(t, filepath.Join(skillDir(root), "SKILL.md"))
	if !strings.Contains(skill, "\nname: agent-session\n") {
		t.Error("SKILL.md has no name in its frontmatter")
	}
	if !strings.Contains(skill, "\ndescription: ") {
		t.Error("SKILL.md has no description; omp would not load it")
	}
	// The whole reason omp does not reuse pi's skill: omp has MCP tools.
	if !strings.Contains(skill, "session.record") {
		t.Error("SKILL.md never names an MCP tool, so it reads like the pi (CLI-only) skill")
	}

	server := mcpServer(t, MCPPath(root))
	if server["command"] != bin {
		t.Errorf("mcp.json command is %v, want %q", server["command"], bin)
	}
	if got, want := server["type"], "stdio"; got != want {
		t.Errorf("mcp.json type is %v, want %q", got, want)
	}
	env, _ := server["env"].(map[string]any)
	if env["AGENT_SESSION_AGENT"] != "omp" {
		t.Errorf("mcp.json does not attribute writes to omp: env is %v", env)
	}
}

// TestCommandsLandWhereOmpLooks pins the two directories that decide whether omp
// ever sees any of this: project resources live under .omp, user resources under
// ~/.omp/agent. A file in the wrong one is a file omp never reads.
func TestCommandsLandWhereOmpLooks(t *testing.T) {
	if got, want := UserRoot("/home/u"), "/home/u/.omp/agent"; got != want {
		t.Errorf("UserRoot = %q, want %q", got, want)
	}
	if got, want := ProjectRoot("/w/proj"), "/w/proj/.omp"; got != want {
		t.Errorf("ProjectRoot = %q, want %q", got, want)
	}

	root := filepath.Join(t.TempDir(), ".omp")
	if err := EnsureCommands(root); err != nil {
		t.Fatalf("ensure commands: %v", err)
	}
	body := read(t, filepath.Join(root, "commands", "agent-session.md"))
	if !strings.HasPrefix(body, "---\n") {
		t.Error("slash command does not start with frontmatter, omp will not parse it")
	}
}

// TestUserRootFollowsActiveProfile: omp resolves its user agent directory through
// the active profile, so wiring ~/.omp/agent for a user whose shell exports
// OMP_PROFILE writes files omp never reads — and doctor then reports the default
// path as missing forever.
func TestUserRootFollowsActiveProfile(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("PI_PROFILE", "")

	t.Setenv("OMP_PROFILE", "work")
	if got, want := UserRoot("/home/u"), "/home/u/.omp/profiles/work/agent"; got != want {
		t.Errorf("UserRoot with OMP_PROFILE=work = %q, want %q", got, want)
	}

	// "default" is the unnamed profile, which lives at the plain agent dir.
	t.Setenv("OMP_PROFILE", "default")
	if got, want := UserRoot("/home/u"), "/home/u/.omp/agent"; got != want {
		t.Errorf("UserRoot with OMP_PROFILE=default = %q, want %q", got, want)
	}

	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "legacy")
	if got, want := UserRoot("/home/u"), "/home/u/.omp/profiles/legacy/agent"; got != want {
		t.Errorf("UserRoot with PI_PROFILE=legacy = %q, want %q", got, want)
	}

	// The explicit agent directory wins over any profile.
	t.Setenv("PI_CODING_AGENT_DIR", "/opt/omp-agent")
	if got, want := UserRoot("/home/u"), "/opt/omp-agent"; got != want {
		t.Errorf("UserRoot with PI_CODING_AGENT_DIR = %q, want %q", got, want)
	}
}

func TestEnsureMCPKeepsOtherServers(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"mcpServers":{"mine":{"command":"my-server"}},"disabledServers":["noisy"]}`
	if err := os.WriteFile(MCPPath(root), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMCP(root, "agent-session"); err != nil {
		t.Fatalf("ensure mcp: %v", err)
	}

	config := readJSON(t, MCPPath(root))
	servers, _ := config["mcpServers"].(map[string]any)
	if _, ok := servers["mine"]; !ok {
		t.Error("registering agent-session dropped the user's own MCP server")
	}
	if _, ok := config["disabledServers"]; !ok {
		t.Error("registering agent-session dropped the disabledServers list")
	}

	if err := RemoveMCP(root); err != nil {
		t.Fatalf("remove mcp: %v", err)
	}
	config = readJSON(t, MCPPath(root))
	servers, _ = config["mcpServers"].(map[string]any)
	if _, ok := servers["agent-session"]; ok {
		t.Error("agent-session survived RemoveMCP")
	}
	if _, ok := servers["mine"]; !ok {
		t.Error("RemoveMCP took the user's own MCP server with it")
	}
}

// TestEnsureMCPRefusesUnparseableConfig is the case where being permissive costs
// the user real work: a file with a typo in it still holds their servers.
func TestEnsureMCPRefusesUnparseableConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	broken := `{"mcpServers":{"mine":{"command":"my-server"},}}`
	if err := os.WriteFile(MCPPath(root), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMCP(root, "agent-session"); err == nil {
		t.Fatal("EnsureMCP overwrote a config it could not parse")
	}
	if got := read(t, MCPPath(root)); got != broken {
		t.Errorf("the unparseable config was modified:\n%s", got)
	}
}

// TestEnsureMCPKeepsKeysOnOurOwnEntry is the re-run case: `init` is documented as
// safe to repeat, so it must not re-enable a server the user disabled or drop the
// knobs they tuned on it.
func TestEnsureMCPKeepsKeysOnOurOwnEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := EnsureMCP(root, "agent-session"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	config := readJSON(t, MCPPath(root))
	servers, _ := config["mcpServers"].(map[string]any)
	entry, _ := servers["agent-session"].(map[string]any)
	entry["enabled"] = false
	entry["timeout"] = float64(120000)
	entry["env"] = map[string]any{"AGENT_SESSION_AGENT": "omp", "AGENT_SESSION_LOG": "debug"}
	servers["agent-session"] = entry
	config["mcpServers"] = servers
	if err := agent.WriteJSONConfig(MCPPath(root), config); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMCP(root, "/new/path/agent-session"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	entry = mcpServer(t, MCPPath(root))
	if entry["command"] != "/new/path/agent-session" {
		t.Errorf("re-running setup did not update the binary path: %v", entry["command"])
	}
	if enabled, ok := entry["enabled"].(bool); !ok || enabled {
		t.Error("re-running setup silently re-enabled a server the user disabled")
	}
	if entry["timeout"] != float64(120000) {
		t.Errorf("re-running setup dropped the user's timeout: %v", entry["timeout"])
	}
	env, _ := entry["env"].(map[string]any)
	if env["AGENT_SESSION_LOG"] != "debug" {
		t.Errorf("re-running setup dropped a user env key: %v", env)
	}
	if env["AGENT_SESSION_AGENT"] != "omp" {
		t.Errorf("attribution env key lost: %v", env)
	}
}

// TestRemoveResourcesSurvivesUnparseableMCP: refusing to rewrite a broken file is
// right, but uninstall still has to remove the files we do own — otherwise the
// lifecycle extension keeps running after the user removed the integration.
func TestRemoveResourcesSurvivesUnparseableMCP(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}
	broken := `{"mcpServers":{"agent-session":{"command":"x"},}}`
	if err := os.WriteFile(MCPPath(root), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveResources(root); err == nil {
		t.Error("RemoveResources hid the unparseable mcp.json instead of reporting it")
	}
	if got := read(t, MCPPath(root)); got != broken {
		t.Errorf("the unparseable config was rewritten:\n%s", got)
	}
	for _, path := range []string{
		ExtensionPath(root),
		filepath.Join(skillDir(root), "SKILL.md"),
		filepath.Join(CommandsDir(root), "agent-session.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall because mcp.json could not be parsed", path)
		}
	}
}

// TestRemoveMCPDeletesFileItCreated keeps uninstall from leaving a file behind
// that exists only to hold a schema URL.
func TestRemoveMCPDeletesFileItCreated(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := EnsureMCP(root, "agent-session"); err != nil {
		t.Fatalf("ensure mcp: %v", err)
	}
	if err := RemoveMCP(root); err != nil {
		t.Fatalf("remove mcp: %v", err)
	}
	if _, err := os.Stat(MCPPath(root)); !os.IsNotExist(err) {
		t.Error("mcp.json survived uninstall with nothing but a schema line in it")
	}
	// Re-runnable: `plugin uninstall` may be called twice.
	if err := RemoveMCP(root); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}

func TestEnsureResourcesIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	extBefore := read(t, ExtensionPath(root))
	mcpBefore := read(t, MCPPath(root))
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if after := read(t, ExtensionPath(root)); after != extBefore {
		t.Error("re-running setup changed the extension")
	}
	if after := read(t, MCPPath(root)); after != mcpBefore {
		t.Error("re-running setup changed mcp.json")
	}
}

// TestEnsureResourcesLeavesForeignFiles covers the case that actually costs a
// user something: a hand-written extension of the same name.
func TestEnsureResourcesLeavesForeignFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	const mine = "// my own extension\n"
	if err := os.MkdirAll(filepath.Dir(ExtensionPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ExtensionPath(root), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}
	if got := read(t, ExtensionPath(root)); got != mine {
		t.Errorf("setup overwrote an extension it does not own:\n%s", got)
	}

	if err := RemoveResources(root); err != nil {
		t.Fatalf("remove resources: %v", err)
	}
	if got := read(t, ExtensionPath(root)); got != mine {
		t.Error("uninstall removed an extension it does not own")
	}
}

func TestRemoveResources(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".omp")
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}
	if err := RemoveResources(root); err != nil {
		t.Fatalf("remove resources: %v", err)
	}

	gone := []string{
		ExtensionPath(root),
		filepath.Join(skillDir(root), "SKILL.md"),
		MCPPath(root),
		filepath.Join(CommandsDir(root), "agent-session.md"),
	}
	for _, path := range gone {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", path)
		}
	}

	// Removing twice must not fail: `plugin uninstall` is re-runnable.
	if err := RemoveResources(root); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}

func TestAdapterUsesProjectScopePaths(t *testing.T) {
	project := t.TempDir()
	a := NewAdapter(project)

	if a.Name() != "omp" {
		t.Errorf("adapter name is %q", a.Name())
	}
	if found, _ := a.Detect(context.Background()); found {
		t.Error("detected omp in a project with no .omp directory")
	}

	if err := a.Configure(context.Background(), "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Project scope is .omp/, user scope is ~/.omp/agent/ — the asymmetry is
	// omp's, and getting it wrong installs files omp never looks at.
	for _, rel := range []string{
		filepath.Join(".omp", "mcp.json"),
		filepath.Join(".omp", "extensions", "agent-session.ts"),
		filepath.Join(".omp", "skills", "agent-session", "SKILL.md"),
		filepath.Join(".omp", "commands", "agent-session.md"),
	} {
		if _, err := os.Stat(filepath.Join(project, rel)); err != nil {
			t.Errorf("%s missing after install: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".omp", "agent")); err == nil {
		t.Error("project scope must not nest under .omp/agent; that is the user-scope layout")
	}
	if found, _ := a.Detect(context.Background()); !found {
		t.Error("omp not detected after install")
	}

	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".omp", "extensions", "agent-session.ts")); !os.IsNotExist(err) {
		t.Error("extension survived uninstall")
	}
}

func mcpServer(t *testing.T, path string) map[string]any {
	t.Helper()
	config := readJSON(t, path)
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no mcpServers map: %v", path, config)
	}
	server, ok := servers["agent-session"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no agent-session server: %v", path, servers)
	}
	return server
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return config
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
