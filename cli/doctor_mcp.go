package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/omp"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/pi"
	"github.com/anaknegeri/agent-session/internal/infrastructure/mcp"
	"github.com/anaknegeri/agent-session/pkg/logger"
	"github.com/anaknegeri/agent-session/pkg/port"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the Agent Session installation and project state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok := true
			check := func(name string, err error) {
				if err != nil {
					ok = false
					fmt.Printf("✗ %s: %v\n", name, err)
					return
				}
				fmt.Printf("✓ %s\n", name)
			}

			app, err := bootstrap.Open(".")
			check("project initialized", err)
			if err != nil {
				return nil
			}

			session, err := currentSession(app)
			check("active session", err)
			if err != nil {
				return nil
			}
			fmt.Printf("  session: %s (%s)\n", session.ID, session.Status)

			check("sqlite store", app.Check())
			check("git workspace", app.WorkspaceCheck())

			fmt.Println()
			ok = checkUserScope() && ok

			if !ok {
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
}

// checkUserScope verifies that installed agent CLIs have agent-session wired at
// user scope (the `init` default), mirroring what installAgentGlobal writes.
// Agents without a user-scope config (cline) are reported but not treated as a
// failure, since they can only be wired per-project.
func checkUserScope() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("✗ user scope: %v\n", err)
		return false
	}

	ok := true
	for _, a := range agent.DetectInstalled() {
		if !a.Present {
			continue
		}
		wired, detail := userScopeWired(home, a.Name)
		if wired {
			fmt.Printf("✓ %s wired at user scope\n", a.Name)
			continue
		}
		if detail == "" {
			fmt.Printf("- %s: no user scope, per-project only (init --only %s)\n", a.Name, a.Name)
			continue
		}
		ok = false
		// A detail that already names its own remedy replaces the generic one:
		// telling a reader to re-run `init` for a server they disabled in the agent
		// sends them after a command that deliberately will not re-enable it.
		if strings.Contains(detail, "run `") {
			fmt.Printf("✗ %s not wired at user scope: %s\n", a.Name, detail)
			continue
		}
		fmt.Printf("✗ %s not wired at user scope: %s\n  run `agent-session init --only %s`\n", a.Name, detail, a.Name)
	}
	reportCodexHookApproval()
	return ok
}

// reportCodexHookApproval reminds the reader that installed Codex hooks stay
// inert until they approve them once in Codex itself. It is not a pass/fail
// check: the approval is recorded as a per-hook trusted_hash under [hooks.state]
// in config.toml, keyed by an identifier agent-session cannot reproduce, so
// claiming to know the state either way would be a guess.
func reportCodexHookApproval() {
	if _, err := exec.LookPath("codex"); err != nil {
		return
	}
	installed, err := codex.NewAdapter().HooksInstalled()
	if err != nil || !installed {
		return
	}
	fmt.Printf("- codex hooks are installed; they run only after you approve them once in `codex`\n" +
		"  (until then: MCP tools work, automatic resume/checkpoint does not)\n")
}

func userScopeWired(home, name string) (bool, string) {
	switch name {
	case "claude":
		return claudeUserScopeWired(home)
	case "opencode":
		return opencodeUserScopeWired(home)
	case "cursor":
		return cursorUserScopeWired(home)
	case "codex":
		return codexUserScopeWired(home)
	case "pi":
		return piUserScopeWired(home)
	case "omp":
		return ompUserScopeWired(home)
	case "cline":
		return false, ""
	}
	return false, "unknown agent"
}

// claudeUserScopeWired checks both halves of what installClaudeGlobal writes: the
// lifecycle hooks in ~/.claude/settings.json and the user-scope MCP registration
// `claude mcp add --scope user` lands in ~/.claude.json. Hooks alone were enough
// for a ✓, so an install with no agent-session server — no session tools at all —
// read as healthy.
func claudeUserScopeWired(home string) (bool, string) {
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "missing ~/.claude/settings.json"
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, "invalid ~/.claude/settings.json"
	}
	if !claude.HasAgentSessionHooks(settings) {
		return false, "no agent-session SessionStart/Stop hooks"
	}
	return claudeUserMCPRegistered(home)
}

func claudeUserMCPRegistered(home string) (bool, string) {
	path := filepath.Join(home, ".claude.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "missing ~/.claude.json — no user-scope MCP server"
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false, "invalid ~/.claude.json"
	}
	servers, _ := config["mcpServers"].(map[string]any)
	if _, ok := servers["agent-session"]; !ok {
		return false, "no user-scope mcpServers.agent-session entry in ~/.claude.json"
	}
	return true, ""
}

func opencodeUserScopeWired(home string) (bool, string) {
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "missing ~/.config/opencode/opencode.json"
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false, "invalid opencode.json"
	}
	mcpServers, _ := config["mcp"].(map[string]any)
	if _, ok := mcpServers["agent-session"]; !ok {
		return false, "no mcp.agent-session entry"
	}
	return true, ""
}

func cursorUserScopeWired(home string) (bool, string) {
	path := filepath.Join(home, ".cursor", "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "missing ~/.cursor/mcp.json"
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false, "invalid ~/.cursor/mcp.json"
	}
	mcpServers, _ := config["mcpServers"].(map[string]any)
	if _, ok := mcpServers["agent-session"]; !ok {
		return false, "no mcpServers.agent-session entry"
	}
	return true, ""
}

// codexUserScopeWired resolves the config through the adapter rather than
// assuming ~/.codex, so a reader who moved Codex with CODEX_HOME is not told
// their working setup is missing. It checks the hooks too: they are half of what
// installCodex writes, and without them resume and checkpoint only happen if the
// model chooses to call the tools.
func codexUserScopeWired(home string) (bool, string) {
	dir, err := codex.ConfigDir()
	if err != nil {
		dir = filepath.Join(home, ".codex")
	}
	path := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("missing %s", path)
	}
	// `codex mcp add` writes the bare header, but a hand-edited config may quote
	// the server name; both declare the same table.
	if !strings.Contains(string(data), "[mcp_servers.agent-session]") &&
		!strings.Contains(string(data), `[mcp_servers."agent-session"]`) {
		return false, "no [mcp_servers.agent-session] section"
	}
	hooks := filepath.Join(dir, "hooks.json")
	installed, err := codex.NewAdapter().HooksInstalled()
	if err != nil {
		return false, fmt.Sprintf("cannot read %s: %v", hooks, err)
	}
	if !installed {
		return false, fmt.Sprintf("no agent-session SessionStart/Stop hooks in %s", hooks)
	}
	return true, ""
}

// piUserScopeWired has no MCP entry to look for: pi ships no MCP client, so the
// lifecycle extension is the whole wiring.
func piUserScopeWired(home string) (bool, string) {
	path := pi.ExtensionPath(pi.UserRoot(home))
	if _, err := os.Stat(path); err != nil {
		return false, fmt.Sprintf("missing %s", path)
	}
	return true, ""
}

// ompUserScopeWired resolves the same agent directory omp does (honoring
// PI_CODING_AGENT_DIR and OMP_PROFILE/PI_PROFILE), then checks that the server is
// both registered and *reachable*: omp's `disabledServers` denylist and a
// `/mcp disable`-written `enabled: false` each hide a registered server
// completely, which is exactly the state a health check exists to catch.
func ompUserScopeWired(home string) (bool, string) {
	root := omp.UserRoot(home)
	path := omp.MCPPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("missing %s", path)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Sprintf("invalid %s", path)
	}
	mcpServers, _ := config["mcpServers"].(map[string]any)
	entry, ok := mcpServers["agent-session"].(map[string]any)
	if !ok {
		if _, present := mcpServers["agent-session"]; !present {
			return false, "no mcpServers.agent-session entry"
		}
		return false, "mcpServers.agent-session is not an object"
	}
	if enabled, set := entry["enabled"].(bool); set && !enabled {
		return false, "mcpServers.agent-session has enabled: false — run `/mcp enable agent-session` in omp"
	}
	disabled, _ := config["disabledServers"].([]any)
	for _, name := range disabled {
		if s, _ := name.(string); s == "agent-session" {
			return false, "agent-session is in disabledServers — run `/mcp enable agent-session` in omp"
		}
	}
	if _, err := os.Stat(omp.ExtensionPath(root)); err != nil {
		return false, "MCP server registered but no lifecycle extension (no automatic resume/checkpoint)"
	}
	return true, ""
}

func newMCPCmd() *cobra.Command {
	var transport, addr string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server (stdio or streamable-http)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := bootstrap.ResolveRoot(".")
			if err != nil {
				return err
			}
			server := mcp.New(root, logger.FromEnv("info"))
			switch transport {
			case "stdio", "":
				return server.ServeStdio()
			case "streamable-http":
				if addr == "auto" {
					ln, p, err := port.Listen(root, port.DefaultBase, port.DefaultSpan)
					if err != nil {
						return err
					}
					defer ln.Close()
					fmt.Fprintf(cmd.ErrOrStderr(), "agent-session MCP listening on http://127.0.0.1:%d/mcp\n", p)
					return server.ServeStreamableHTTP(ln)
				}
				ln, err := net.Listen("tcp", addr)
				if err != nil {
					return err
				}
				defer ln.Close()
				return server.ServeStreamableHTTP(ln)
			default:
				return fmt.Errorf("unknown transport %q", transport)
			}
		},
	}
	cmd.Flags().StringVar(&transport, "transport", "stdio", "stdio | streamable-http")
	cmd.Flags().StringVar(&addr, "addr", "auto", "HTTP listen address (streamable-http). auto picks a unique port per project")
	return cmd
}
