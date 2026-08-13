package cline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "cline" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	for _, p := range []string{".clinerules", ".cline", filepath.Join(".vscode", "settings.json")} {
		if _, err := os.Stat(filepath.Join(a.projectRoot, p)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	if err := a.writeRules(); err != nil {
		return err
	}
	return a.writeVSCodeMCP(mcpCommand)
}

const clinerulesContent = `<!-- ` + agent.ManagedMarker + ` -->

# Agent Session (mandatory workflow)

This project uses Agent Session (agent-session) as its session layer. Always follow this workflow:

1. When a session starts, FIRST call the agent-session MCP tools in order:
   - session.get — find the current session
   - context.get — load the current context
   Continue the existing task; do not start from scratch.
   The context summary is a bounded preview for token savings — call
   context.get depth=full whenever you need complete decisions, blockers,
   changed files, or the full event list.
2. Record work as you go:
   - task.create / task.update — track the current task
   - decision.create — record architectural decisions with a reason
   - blocker.create — record blockers
   - event.append — record test results (test.failed / test.passed) and file.changed
3. Before finishing (Stop / session end), create a checkpoint:
   - session.checkpoint — include the next_action so the next agent can continue.
`

// writeRules installs the mandatory-workflow rule where Cline will read it.
//
// Ownership goes through WriteManaged: a `.clinerules` the user wrote themselves
// has no marker and is left exactly as found.
func (a *Adapter) writeRules() error {
	return agent.WriteManaged(a.rulesPath(), clinerulesContent)
}

// rulesPath resolves the two shapes Cline accepts. A project that already keeps
// `.clinerules` as a directory of markdown files gets ours as one more file in it:
// writing the directory path as a file fails with "is a directory", which used to
// abort Configure before the MCP wiring ran and left cline with neither.
func (a *Adapter) rulesPath() string {
	base := filepath.Join(a.projectRoot, ".clinerules")
	if info, err := os.Stat(base); err == nil && info.IsDir() {
		return filepath.Join(base, "agent-session.md")
	}
	return base
}

func (a *Adapter) writeVSCodeMCP(mcpCommand string) error {
	path := filepath.Join(a.projectRoot, ".vscode", "settings.json")
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		// VS Code accepts JSONC — comments and trailing commas are legal there and
		// encoding/json rejects both — so this is a normal file, not a broken one.
		// Rewriting it would drop every workspace setting the user has, and even a
		// successful rewrite would strip their comments, so we stop and say what to add.
		return fmt.Errorf("%w\n  add this to .vscode/settings.json by hand:\n"+
			`    "cline.mcpServers": {"agent-session": {"command": %q, "args": ["mcp"], "env": {"AGENT_SESSION_AGENT": "cline"}}}`,
			err, mcpCommand)
	}
	entry := agent.Section(agent.Section(config, "cline.mcpServers"), "agent-session")
	entry["command"] = mcpCommand
	entry["args"] = []any{"mcp"}
	entry["disabled"] = false
	if _, ok := entry["autoApprove"]; !ok {
		entry["autoApprove"] = []any{}
	}
	agent.Section(entry, "env")["AGENT_SESSION_AGENT"] = "cline"
	return agent.WriteJSONConfig(path, config)
}

func (a *Adapter) Install(ctx context.Context) error { return nil }

// Uninstall removes the rule before the MCP entry, because the rule is the half
// that keeps instructing the agent: leaving it behind means an uninstalled
// agent-session still tells Cline to call tools that are no longer registered.
func (a *Adapter) Uninstall(ctx context.Context) error {
	if err := agent.RemoveManaged(a.rulesPath()); err != nil {
		return err
	}
	return a.uninstallVSCodeMCP()
}

func (a *Adapter) uninstallVSCodeMCP() error {
	path := filepath.Join(a.projectRoot, ".vscode", "settings.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	servers, ok := config["cline.mcpServers"].(map[string]any)
	if !ok {
		return nil
	}
	delete(servers, "agent-session")
	if len(servers) == 0 {
		delete(config, "cline.mcpServers")
	}
	// A settings.json that held nothing but our entry is ours to remove; a file
	// with any other workspace setting in it stays.
	if len(config) == 0 {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return agent.WriteJSONConfig(path, config)
}

var _ agent.Adapter = (*Adapter)(nil)
