package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/pkg/update"
)

func newUpdateCmd() *cobra.Command {
	var checkOnly bool
	var force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and apply the latest release",
		Long: `Check for and apply the latest release from GitHub.

Without flags, downloads the newest release and replaces the running binary.
Use --check to only report whether an update is available.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			info, err := update.Check(ctx, update.DefaultRepo)
			if err != nil {
				return fmt.Errorf("check update: %w", err)
			}

			if !info.HasUpdate {
				fmt.Printf("agent-session %s is up to date.\n", info.Current)
				return nil
			}

			fmt.Printf("Update available: %s → %s\n", info.Current, info.Latest)
			if checkOnly {
				fmt.Printf("Run `agent-session update` to apply.\n")
				return nil
			}

			newVersion, err := update.SelfUpdate(ctx, update.DefaultRepo, force)
			if err != nil {
				return fmt.Errorf("update failed: %w", err)
			}
			fmt.Printf("Updated to %s (agent-session + agent-session-mcp). Restart any running MCP servers to pick up the change.\n", newVersion)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Only report whether an update is available")
	cmd.Flags().BoolVar(&force, "force", false, "Re-download even when already up to date")
	return cmd
}
