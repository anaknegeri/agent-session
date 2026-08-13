package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cline"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/commands"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cursor"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/omp"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/opencode"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/pi"
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
	// pi is deliberately absent: its prompts are installed by the pi package,
	// because the universal set instructs the agent to call MCP tools and pi has
	// no MCP client. See internal/infrastructure/agent/pi.
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
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	// `claude mcp add` leaves an existing entry alone, so a registration made
	// before the binary moved would keep pointing at a path that no longer exists
	// — and Claude Code reports nothing, it just has no session tools. Drop the
	// stale entry first so the add below rewrites it, the way every other adapter
	// rewrites `command` on re-run.
	registered, err := claude.UserMCPCommand(home)
	if err != nil {
		return err
	}
	if registered != "" && registered != bin {
		remove := exec.Command("claude", "mcp", "remove", "--scope", "user", "agent-session")
		if out, rmErr := remove.CombinedOutput(); rmErr != nil {
			return fmt.Errorf("claude mcp remove (stale %s): %v: %s", registered, rmErr, strings.TrimSpace(string(out)))
		}
	}
	cmd := exec.Command("claude", "mcp", "add", "--scope", "user", "agent-session", "--", bin, "mcp")
	if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("claude mcp add (user scope): %v: %s", err, strings.TrimSpace(string(out)))
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
	if err := a.Install(ctx()); err != nil {
		red("✗ codex hooks: %v\n", err)
		return
	}
	green("✓ codex: mcp_servers.agent-session + hooks written to ~/.codex/hooks.json\n")
	// Codex discovers a newly written hook but will not run it until the user has
	// approved it once, which records a trusted_hash in config.toml. Writing that
	// hash here would defeat the gate that stops an installer from wiring shell
	// commands into someone's agent, so setup spells the manual step out instead
	// of leaving the hooks looking wired while they are silently inert.
	yellow("  ! automatic resume/checkpoint is not active yet — Codex hooks need a one-time approval:\n")
	yellow("      1. run `codex` in a project that has .agent/\n")
	yellow("      2. approve the agent-session hooks when Codex asks (once per machine)\n")
	yellow("    Until then the MCP tools work, but nothing is resumed or checkpointed on its own.\n")
	yellow("    agent-session cannot approve on your behalf: that gate is what stops an installer\n")
	yellow("    from wiring shell commands into your agent.\n")
}

func installCursor(dir, bin string) {
	a := cursor.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ cursor: %v\n", err)
		return
	}
	green("✓ cursor: .cursor/mcp.json + .cursor/rules/agent-session.mdc\n")
}

// installPi wires pi at project scope: .pi/extensions + skill + prompts. Those
// stay inert until the user trusts the project in pi, which is why the caller
// prints the manual step.
func installPi(dir, bin string) {
	a := pi.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ pi: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		red("✗ pi: %v\n", err)
		return
	}
	green("✓ pi: .pi/extensions/agent-session.ts + skill + prompts (no MCP — pi ships none)\n")
	yellow("  ! project-local pi resources load only after you trust this project in pi:\n")
	yellow("      run `pi` here and approve the project once\n")
	yellow("    agent-session cannot approve on your behalf: that gate is what stops an installer\n")
	yellow("    from wiring shell commands into your agent. Use user scope (no --project) to skip it.\n")
}

// installPiGlobal wires pi at user scope (~/.pi/agent), which needs no project
// trust: the extension guards itself by looking for .agent/ before doing
// anything, so it is silent in projects that do not use agent-session.
func installPiGlobal(bin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	return pi.EnsureResources(pi.UserRoot(home), bin)
}

// uninstallPiGlobal reverses installPiGlobal.
func uninstallPiGlobal() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	return pi.RemoveResources(pi.UserRoot(home))
}

// installOmp wires omp at project scope: .omp/mcp.json + extension + skill +
// commands. Unlike pi, omp needs no per-project approval — project resources load
// on the next session, so the files are live as soon as they are written.
func installOmp(dir, bin string) {
	a := omp.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ omp: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		red("✗ omp: %v\n", err)
		return
	}
	green("✓ omp: .omp/mcp.json + .omp/extensions/agent-session.ts + skill + commands\n")
}

// installOmpGlobal wires omp at user scope (~/.omp/agent), the default profile's
// agent directory. The extension guards itself by looking for .agent/ before
// doing anything, so it stays silent in projects that do not use agent-session.
// A named profile (`omp --profile x`) reads ~/.omp/profiles/x/agent instead and
// is not covered by this.
func installOmpGlobal(bin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	return omp.EnsureResources(omp.UserRoot(home), bin)
}

// uninstallOmpGlobal reverses installOmpGlobal.
func uninstallOmpGlobal() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	return omp.RemoveResources(omp.UserRoot(home))
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
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	entry := agent.Section(agent.Section(config, "mcp"), "agent-session")
	entry["type"] = "local"
	entry["command"] = []any{bin, "mcp"}
	entry["enabled"] = true
	agent.Section(entry, "environment")["AGENT_SESSION_AGENT"] = "opencode"
	opencode.MergeGlobalInstructions(config)

	if err := agent.WriteJSONConfig(path, config); err != nil {
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
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return uninstallCommands("opencode")
	}
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	if mcpServers, ok := config["mcp"].(map[string]any); ok {
		delete(mcpServers, "agent-session")
		if len(mcpServers) == 0 {
			delete(config, "mcp")
		}
	}
	opencode.RemoveGlobalInstructions(config)

	if err := agent.WriteJSONConfig(path, config); err != nil {
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
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	entry := agent.Section(agent.Section(config, "mcpServers"), "agent-session")
	entry["command"] = bin
	entry["args"] = []any{"mcp"}
	agent.Section(entry, "env")["AGENT_SESSION_AGENT"] = "cursor"

	if err := agent.WriteJSONConfig(path, config); err != nil {
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
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return uninstallCommands("cursor")
	}
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return uninstallCommands("cursor")
	}
	delete(servers, "agent-session")
	if len(servers) == 0 {
		delete(config, "mcpServers")
	}

	if err := agent.WriteJSONConfig(path, config); err != nil {
		return err
	}
	return uninstallCommands("cursor")
}
