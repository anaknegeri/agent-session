package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cline"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cursor"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/opencode"
)

// installClaude wires Claude Code via project .mcp.json + CLAUDE.md + hooks.
func installClaude(dir, bin string) {
	a := claude.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ claude: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		red("✗ claude: %v\n", err)
		return
	}
	green("✓ claude: .mcp.json + .claude/CLAUDE.md + hooks (auto resume/checkpoint)\n")
}

func installOpenCode(dir, bin string) {
	a := opencode.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ opencode: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		red("✗ opencode: %v\n", err)
		return
	}
	green("✓ opencode: opencode.json (mcp + always-on instructions)\n")
}

func installCodex(bin string) {
	if _, err := exec.LookPath("codex"); err != nil {
		yellow("- codex: CLI not found, skipping (AGENTS.md covers it)\n")
		return
	}
	a := codex.NewAdapter()
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ codex: %v\n", err)
		return
	}
	green("✓ codex: mcp_servers.agent-session registered\n")
}

func installCursor(dir, bin string) {
	a := cursor.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ cursor: %v\n", err)
		return
	}
	green("✓ cursor: .cursor/mcp.json + .cursor/rules/agent-session.mdc\n")
}

func installCline(dir, bin string) {
	a := cline.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ cline: %v\n", err)
		return
	}
	green("✓ cline: .clinerules + .vscode/settings.json (cline.mcpServers)\n")
}

// installOpenCodeGlobal registers agent-session in the user-level
// ~/.config/opencode/opencode.json so it is available in every project.
func installOpenCodeGlobal(bin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
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
		"command": []string{bin, "mcp"},
		"enabled": true,
		"environment": map[string]string{
			"AGENT_SESSION_AGENT": "opencode",
		},
	}
	config["mcp"] = mcpServers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode.json: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

// installCursorGlobal registers agent-session in the user-level ~/.cursor/mcp.json.
func installCursorGlobal(bin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create .cursor dir: %w", err)
	}
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
		"command": bin,
		"args":    []string{"mcp"},
		"env": map[string]string{
			"AGENT_SESSION_AGENT": "cursor",
		},
	}
	config["mcpServers"] = mcpServers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cursor mcp.json: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}
