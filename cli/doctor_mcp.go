package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
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
		fmt.Printf("✗ %s not wired at user scope: %s\n  run `agent-session init --only %s`\n", a.Name, detail, a.Name)
	}
	return ok
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
	case "cline":
		return false, ""
	}
	return false, "unknown agent"
}

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

func codexUserScopeWired(home string) (bool, string) {
	path := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "missing ~/.codex/config.toml"
	}
	if !strings.Contains(string(data), "[mcp_servers.agent-session]") {
		return false, "no [mcp_servers.agent-session] section"
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
