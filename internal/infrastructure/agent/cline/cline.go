package cline

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

const clinerulesContent = `# Agent Session (mandatory workflow)

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

func (a *Adapter) writeRules() error {
	path := filepath.Join(a.projectRoot, ".clinerules")
	content, err := os.ReadFile(path)
	if err == nil && len(content) > 0 {
		return nil
	}
	return os.WriteFile(path, []byte(clinerulesContent), 0o644)
}

func (a *Adapter) writeVSCodeMCP(mcpCommand string) error {
	vscodeDir := filepath.Join(a.projectRoot, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return fmt.Errorf("create .vscode dir: %w", err)
	}
	path := filepath.Join(vscodeDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte("{}")
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		config = map[string]any{}
	}

	mcpServers, _ := config["cline.mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	mcpServers["agent-session"] = map[string]any{
		"command": mcpCommand,
		"args":    []string{"mcp"},
		"env": map[string]string{
			"AGENT_SESSION_AGENT": "cline",
		},
		"disabled": false,
		"autoApprove": []string{},
	}
	config["cline.mcpServers"] = mcpServers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

func (a *Adapter) Install(ctx context.Context) error  { return nil }
func (a *Adapter) Uninstall(ctx context.Context) error {
	path := filepath.Join(a.projectRoot, ".vscode", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	mcpServers, _ := config["cline.mcpServers"].(map[string]any)
	if mcpServers == nil {
		return nil
	}
	delete(mcpServers, "agent-session")
	if len(mcpServers) == 0 {
		delete(config, "cline.mcpServers")
	} else {
		config["cline.mcpServers"] = mcpServers
	}
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

var _ agent.Adapter = (*Adapter)(nil)
