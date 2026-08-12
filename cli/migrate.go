package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Remove old per-project agent configs and re-wire at user scope",
		Long: `Remove per-project agent config files (.claude/, .cursor/, .clinerules,
.vscode/, .mcp.json, opencode.json) left over from older versions of
agent-session init, then re-wire the agents you use at user scope so the
project stays clean.

Idempotent — safe to re-run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := cwd()
			bin := selfPath()

			removed := removeOldProjectConfigs(dir)
			if removed == 0 {
				fmt.Println("No old per-project agent configs found — project is already clean.")
			}

			fmt.Println("\nRe-wiring agents at user scope...")
			for _, a := range agent.DetectInstalled() {
				if a.Present {
					installAgentGlobal(bin, a.Name)
				}
			}
			fmt.Println("\nMigration complete.")
			return nil
		},
	}
	return cmd
}

// removeOldProjectConfigs deletes the per-project agent config files/dirs that
// older versions of `init` created. Returns the count of items removed.
func removeOldProjectConfigs(dir string) int {
	targets := []string{
		".claude",
		".cursor",
		".vscode",
		".clinerules",
		".mcp.json",
		"opencode.json",
	}
	removed := 0
	for _, t := range targets {
		path := filepath.Join(dir, t)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := os.RemoveAll(path); err != nil {
				red("✗ could not remove %s/: %v\n", t, err)
				continue
			}
		} else {
			if err := os.Remove(path); err != nil {
				red("✗ could not remove %s: %v\n", t, err)
				continue
			}
		}
		green("✓ removed %s\n", t)
		removed++
	}
	return removed
}
