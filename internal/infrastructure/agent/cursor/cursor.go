package cursor

import (
	"context"
	"encoding/json"
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

func (a *Adapter) Name() string { return "cursor" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	cursorDir := filepath.Join(a.projectRoot, ".cursor")
	_, err := os.Stat(cursorDir)
	return err == nil, nil
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	cursorDir := filepath.Join(a.projectRoot, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return fmt.Errorf("create .cursor dir: %w", err)
	}

	if err := a.writeMCPConfig(cursorDir, mcpCommand); err != nil {
		return err
	}
	if err := a.writeRules(cursorDir); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) writeMCPConfig(cursorDir, mcpCommand string) error {
	path := filepath.Join(cursorDir, "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte("{}")
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		config = map[string]any{}
	}

	mcpServers, _ := config["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	mcpServers["agent-session"] = map[string]any{
		"command": mcpCommand,
		"args":    []string{"mcp"},
		"env": map[string]string{
			"AGENT_SESSION_AGENT": "cursor",
		},
	}
	config["mcpServers"] = mcpServers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp.json: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

const rulesContent = `---
description: Agent Session — universal session & handoff layer for AI coding agents
alwaysApply: true
---

` + agentInstructions

const agentInstructions = `This project uses Agent Session (agent-session) as its session layer. Always follow this workflow:

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

func (a *Adapter) writeRules(cursorDir string) error {
	rulesDir := filepath.Join(cursorDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return fmt.Errorf("create rules dir: %w", err)
	}
	path := filepath.Join(rulesDir, "agent-session.mdc")
	content, err := os.ReadFile(path)
	if err == nil && len(content) > 0 {
		return nil
	}
	return os.WriteFile(path, []byte(rulesContent), 0o644)
}

func (a *Adapter) Install(ctx context.Context) error  { return nil }
func (a *Adapter) Uninstall(ctx context.Context) error {
	mcpPath := filepath.Join(a.projectRoot, ".cursor", "mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	mcpServers, _ := config["mcpServers"].(map[string]any)
	if mcpServers == nil {
		return nil
	}
	delete(mcpServers, "agent-session")
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp.json: %w", err)
	}
	return os.WriteFile(mcpPath, out, 0o644)
}

var _ agent.Adapter = (*Adapter)(nil)
