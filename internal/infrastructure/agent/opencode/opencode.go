package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agent-session/agent-session/internal/infrastructure/agent"
)

// Adapter configures OpenCode via project-level opencode.json.
type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "opencode" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	paths := []string{
		filepath.Join(a.projectRoot, "opencode.json"),
		filepath.Join(a.projectRoot, "opencode.jsonc"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	path := filepath.Join(a.projectRoot, "opencode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte("{}")
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		config = map[string]any{}
	}

	mcpServers, _ := config["mcp"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	mcpServers["agent-session"] = map[string]any{
		"type":    "local",
		"command": []string{mcpCommand, "mcp"},
		"enabled": true,
		"environment": map[string]string{
			"AGENT_SESSION_AGENT": "opencode",
		},
	}
	config["mcp"] = mcpServers

	// always-on instructions: read the session at start, checkpoint at end
	agentCfg, _ := config["agent"].(map[string]any)
	if agentCfg == nil {
		agentCfg = map[string]any{}
	}
	agentCfg["instructions"] = map[string]any{
		"system": "This project uses Agent Session. At the start of a session, FIRST call the agent-session MCP tools in order: session.get then context.get, and continue the existing task. Record work with task.create/task.update, decision.create, blocker.create, and event.append for test results. Before finishing, create a checkpoint with session.checkpoint including next_action.",
	}
	config["agent"] = agentCfg

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode.json: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write opencode.json: %w", err)
	}
	return nil
}

func (a *Adapter) Install(ctx context.Context) error {
	return nil
}

func (a *Adapter) Uninstall(ctx context.Context) error {
	path := filepath.Join(a.projectRoot, "opencode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	mcpServers, _ := config["mcp"].(map[string]any)
	if mcpServers == nil {
		return nil
	}
	delete(mcpServers, "agent-session")
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode.json: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

var _ agent.Adapter = (*Adapter)(nil)
