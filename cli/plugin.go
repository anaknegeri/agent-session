package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/agent-session/agent-session/internal/infrastructure/agent/claude"
	"github.com/agent-session/agent-session/internal/infrastructure/agent/codex"
	"github.com/agent-session/agent-session/internal/infrastructure/agent/opencode"
	"github.com/agent-session/agent-session/internal/infrastructure/agent/plugin"
)

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Package and install the Agent Session Agent Plugin",
	}
	cmd.AddCommand(
		newPluginPackCmd(),
		newPluginInstallCmd(),
		newPluginUninstallCmd(),
	)
	return cmd
}

func newPluginPackCmd() *cobra.Command {
	var binary, out, version string
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Build the Agent Plugin package (plugin.json + mcp.json + bin/)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if binary == "" {
				binary = selfPath()
			}
			if out == "" {
				out = "plugin"
			}
			if version == "" {
				version = "0.1.0"
			}
			if err := plugin.Pack(out, binary, version); err != nil {
				return err
			}
			fmt.Printf("Plugin packaged at %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&binary, "binary", "", "Path to the agent-session binary")
	cmd.Flags().StringVarP(&out, "out", "o", "plugin", "Output directory")
	cmd.Flags().StringVar(&version, "version", "0.1.0", "Plugin version")
	return cmd
}

func newPluginInstallCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "install <agent>",
		Short: "Install the MCP server for an agent (claude, codex, opencode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureAgent(args[0], true, scope)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "project", "claude scope: project (.mcp.json) | user (~/.claude.json, always available)")
	return cmd
}

func newPluginUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <agent>",
		Short: "Uninstall the MCP server for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureAgent(args[0], false, "")
		},
	}
}

func configureAgent(name string, install bool, scope string) error {
	bin := selfPath()
	switch name {
	case "claude":
		if install && scope == "user" {
			cmd := exec.Command("claude", "mcp", "add", "--scope", "user", "agent-session", "--", bin, "mcp")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("claude mcp add (user scope): %v: %s", err, out)
			}
			return nil
		}
		a := claude.NewAdapter(cwd())
		if install {
			if err := a.Configure(ctx(), bin); err != nil {
				return err
			}
			return a.Install(ctx())
		}
		return a.Uninstall(ctx())
	case "codex":
		a := codex.NewAdapter()
		if install {
			if err := a.Configure(ctx(), bin); err != nil {
				return err
			}
			return a.Install(ctx())
		}
		return a.Uninstall(ctx())
	case "opencode":
		a := opencode.NewAdapter(cwd())
		if install {
			if err := a.Configure(ctx(), bin); err != nil {
				return err
			}
			return a.Install(ctx())
		}
		return a.Uninstall(ctx())
	}
	return fmt.Errorf("unknown agent %q (claude, codex, opencode)", name)
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "agent-session"
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
