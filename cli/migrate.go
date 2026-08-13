package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cline"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cursor"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/omp"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/opencode"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/pi"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Remove per-project agent wiring and re-wire at user scope",
		Long: `Remove the per-project agent wiring that older versions of ` + "`init`" + ` created
(.mcp.json, .claude/, .cursor/, .vscode/, .clinerules, opencode.json, .pi/, .omp/),
then re-wire the agents you use at user scope so the project stays clean.

Only agent-session's own entries are removed: each adapter's uninstall strips our
MCP server, hooks, rules and commands and leaves every other setting — VS Code
launch configs, Claude permissions, your CLAUDE.md, other MCP servers — in place.

Idempotent — safe to re-run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := cwd()
			bin := selfPath()

			if removeProjectWiring(dir) == 0 {
				fmt.Println("No per-project agent wiring found — project is already clean.")
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

// removeProjectWiring asks every adapter to uninstall itself from dir, and
// returns how many reported work done. It never deletes a config file or
// directory wholesale: `.claude/`, `.cursor/` and `.vscode/` hold the user's own
// agents, commands, rules, launch configs and tasks, and an older version of this
// command removing them outright is the reason this one goes through adapters.
func removeProjectWiring(dir string) int {
	type target struct {
		name    string
		adapter agent.Adapter
		// probes are the files whose disappearance or shrinking proves work.
		probes []string
	}
	targets := []target{
		{"claude", claude.NewAdapter(dir), []string{".mcp.json", ".claude/settings.json", ".claude/CLAUDE.md"}},
		{"cursor", cursor.NewAdapter(dir), []string{".cursor/mcp.json"}},
		{"cline", cline.NewAdapter(dir), []string{".vscode/settings.json", ".clinerules", ".clinerules/agent-session.md"}},
		{"opencode", opencode.NewAdapter(dir), []string{"opencode.json", "opencode.jsonc"}},
		{"pi", pi.NewAdapter(dir), []string{".pi/extensions/agent-session.ts"}},
		{"omp", omp.NewAdapter(dir), []string{".omp/mcp.json", ".omp/extensions/agent-session.ts"}},
	}

	changed := 0
	for _, t := range targets {
		before := fingerprint(dir, t.probes)
		if err := t.adapter.Uninstall(ctx()); err != nil {
			red("✗ %s: %v\n", t.name, err)
			continue
		}
		if fingerprint(dir, t.probes) == before {
			continue
		}
		green("✓ removed %s wiring\n", t.name)
		changed++
	}
	return changed
}

// fingerprint is size-per-probe, which is enough to tell "uninstall changed
// something" from "there was nothing of ours here" without diffing content.
func fingerprint(dir string, probes []string) string {
	var b strings.Builder
	for _, p := range probes {
		info, err := os.Stat(filepath.Join(dir, p))
		if err != nil {
			b.WriteString(p + ":-;")
			continue
		}
		fmt.Fprintf(&b, "%s:%d;", p, info.Size())
	}
	return b.String()
}
