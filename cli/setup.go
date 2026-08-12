package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cline"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/commands"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cursor"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/opencode"
)

// commandsDirFor returns the user-scope slash-command directory for an agent,
// or "" when the agent has no slash-command directory.
func commandsDirFor(agentName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	switch agentName {
	case "claude":
		return filepath.Join(home, ".claude", "commands"), nil
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "commands"), nil
	case "cursor":
		return filepath.Join(home, ".cursor", "commands"), nil
	default:
		return "", nil
	}
}

// installCommands installs slash commands for an agent (user scope). No-op for
// agents without a commands dir.
func installCommands(agentName string) error {
	dir, err := commandsDirFor(agentName)
	if err != nil || dir == "" {
		return err
	}
	_, err = commands.Install(dir)
	return err
}

// uninstallCommands removes slash commands for an agent (user scope).
func uninstallCommands(agentName string) error {
	dir, err := commandsDirFor(agentName)
	if err != nil || dir == "" {
		return err
	}
	_, err = commands.Uninstall(dir)
	return err
}

// installClaudeGlobal registers agent-session in Claude Code at user scope,
// merges the guarded SessionStart/Stop/PreCompact hooks into
// ~/.claude/settings.json, and adds the Agent Session rule to ~/.claude/CLAUDE.md
// (AGENTS.md alone is never read by Claude Code) so state loads and
// checkpoints automatically in every agent-session project on this machine.
func installClaudeGlobal(bin string) error {
	cmd := exec.Command("claude", "mcp", "add", "--scope", "user", "agent-session", "--", bin, "mcp")
	if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("claude mcp add (user scope): %v: %s", err, strings.TrimSpace(string(out)))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	if err := claude.EnsureGlobalHooks(home); err != nil {
		return fmt.Errorf("claude hooks: %w", err)
	}
	if _, err := claude.EnsureGlobalRule(home); err != nil {
		return fmt.Errorf("claude rule: %w", err)
	}
	if err := installCommands("claude"); err != nil {
		return fmt.Errorf("claude commands: %w", err)
	}
	return nil
}

// uninstallClaudeGlobal reverses installClaudeGlobal: deregisters the
// user-scope MCP server and strips the hooks/rule/commands it added, leaving
// any other ~/.claude settings untouched.
func uninstallClaudeGlobal() error {
	cmd := exec.Command("claude", "mcp", "remove", "--scope", "user", "agent-session")
	if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(strings.ToLower(string(out)), "no mcp server named") {
		return fmt.Errorf("claude mcp remove (user scope): %v: %s", err, strings.TrimSpace(string(out)))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	if err := claude.RemoveGlobalHooks(home); err != nil {
		return fmt.Errorf("claude hooks: %w", err)
	}
	if err := claude.RemoveGlobalRule(home); err != nil {
		return fmt.Errorf("claude rule: %w", err)
	}
	if err := uninstallCommands("claude"); err != nil {
		return fmt.Errorf("claude commands: %w", err)
	}
	return nil
}

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
	opencode.MergeGlobalInstructions(config)

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode.json: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	return installCommands("opencode")
}

// uninstallOpenCodeGlobal reverses installOpenCodeGlobal: removes the
// agent-session MCP entry and its always-on instructions from
// ~/.config/opencode/opencode.json, leaving any other config untouched.
func uninstallOpenCodeGlobal() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if mcpServers, ok := config["mcp"].(map[string]any); ok {
		delete(mcpServers, "agent-session")
		if len(mcpServers) == 0 {
			delete(config, "mcp")
		} else {
			config["mcp"] = mcpServers
		}
	}
	opencode.RemoveGlobalInstructions(config)

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode.json: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	return uninstallCommands("opencode")
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
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	return installCommands("cursor")
}

// uninstallCursorGlobal reverses installCursorGlobal: removes the
// agent-session entry from ~/.cursor/mcp.json, leaving any other MCP server
// registration untouched.
func uninstallCursorGlobal() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".cursor", "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}
	delete(mcpServers, "agent-session")
	if len(mcpServers) == 0 {
		delete(config, "mcpServers")
	} else {
		config["mcpServers"] = mcpServers
	}

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cursor mcp.json: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	return uninstallCommands("cursor")
}
