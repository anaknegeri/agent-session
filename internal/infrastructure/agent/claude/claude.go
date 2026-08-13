package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

// Adapter configures Claude Code via a project-level .mcp.json which Claude
// Code discovers automatically.
type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "claude" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	paths := []string{
		filepath.Join(a.projectRoot, ".claude", "settings.json"),
		filepath.Join(a.projectRoot, ".mcp.json"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true, nil
		}
	}
	return false, nil
}

// SettingsPath is the project settings file Claude Code merges over the user one.
func (a *Adapter) SettingsPath() string {
	return filepath.Join(a.projectRoot, ".claude", "settings.json")
}

// RulePath is the project memory file Claude Code reads.
func (a *Adapter) RulePath() string {
	return filepath.Join(a.projectRoot, ".claude", "CLAUDE.md")
}

// MCPPath is the committed per-project MCP config Claude Code discovers.
func (a *Adapter) MCPPath() string { return filepath.Join(a.projectRoot, ".mcp.json") }

// Configure registers the MCP server in .mcp.json, merging into whatever is
// already there. That file is normally committed and shared, so it routinely
// holds other servers: rewriting it from scratch would delete them.
func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	path := a.MCPPath()
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	entry := agent.Section(agent.Section(config, "mcpServers"), "agent-session")
	entry["type"] = "stdio"
	entry["command"] = mcpCommand
	entry["args"] = []any{"mcp"}
	agent.Section(entry, "env")["AGENT_SESSION_AGENT"] = "claude"
	return agent.WriteJSONConfig(path, config)
}

// Install merges the lifecycle hooks into .claude/settings.json and appends the
// Agent Session section to .claude/CLAUDE.md. Both files belong to the project,
// not to us: settings.json commonly carries permissions, model and env, and
// CLAUDE.md is the project's own memory.
func (a *Adapter) Install(ctx context.Context) error {
	if err := EnsureHooks(a.SettingsPath()); err != nil {
		return err
	}
	if _, err := EnsureRule(a.RulePath(), ProjectRule); err != nil {
		return err
	}
	return nil
}

// Uninstall removes exactly what Configure and Install added: our MCP entry, our
// hook entries, our rule section. Files that held something else survive.
func (a *Adapter) Uninstall(ctx context.Context) error {
	if err := a.removeMCP(); err != nil {
		return err
	}
	if err := RemoveHooks(a.SettingsPath()); err != nil {
		return err
	}
	return RemoveRule(a.RulePath(), ProjectRule)
}

func (a *Adapter) removeMCP() error {
	path := a.MCPPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}
	delete(servers, "agent-session")
	if len(servers) == 0 {
		delete(config, "mcpServers")
	}
	if len(config) == 0 {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return agent.WriteJSONConfig(path, config)
}

var _ agent.Adapter = (*Adapter)(nil)
