package claude

import (
	"context"
	"encoding/json"
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

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	config := map[string]any{
		"mcpServers": map[string]any{
			"agent-session": map[string]any{
				"type":    "stdio",
				"command": mcpCommand,
				"args":    []string{"mcp"},
				"env": map[string]string{
					"AGENT_SESSION_AGENT": "claude",
				},
			},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal .mcp.json: %w", err)
	}
	path := filepath.Join(a.projectRoot, ".mcp.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write .mcp.json: %w", err)
	}
	return nil
}

func (a *Adapter) Install(ctx context.Context) error {
	settingsDir := filepath.Join(a.projectRoot, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []map[string]any{
				{
					"matcher": "*",
					"hooks": []map[string]any{
						{
							"type":    "command",
							"command": "agent-session resume --agent claude",
						},
					},
				},
			},
			"Stop": []map[string]any{
				{
					"matcher": "*",
					"hooks": []map[string]any{
						{
							"type":    "command",
							"command": "agent-session checkpoint --label auto",
						},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude settings: %w", err)
	}
	path := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write claude settings: %w", err)
	}

	claudeMD := []byte(`# Agent Session

This project uses Agent Session (agent-session) as its session layer.

- At the start of a session, FIRST call the agent-session MCP tools in order:
  session.get, then context.get. Continue the existing task; do not start from scratch.
- Record work as you go: task.create / task.update, decision.create, blocker.create,
  and event.append for test results (test.failed / test.passed).
- Before finishing (Stop), create a checkpoint with session.checkpoint including next_action.
`)
	if err := os.WriteFile(filepath.Join(settingsDir, "CLAUDE.md"), claudeMD, 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}
	return nil
}

func (a *Adapter) Uninstall(ctx context.Context) error {
	_ = os.Remove(filepath.Join(a.projectRoot, ".mcp.json"))
	_ = os.Remove(filepath.Join(a.projectRoot, ".claude", "settings.json"))
	_ = os.Remove(filepath.Join(a.projectRoot, ".claude", "CLAUDE.md"))
	return nil
}

var _ agent.Adapter = (*Adapter)(nil)
