package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

func newInitCmd() *cobra.Command {
	var only string
	var noAgents bool
	cmd := &cobra.Command{
		Use:     "init",
		Aliases: []string{"setup"},
		Short:   "One-command setup: init the project and wire the agents you use (like git init)",
		Long: "Initializes the project (creates .agent/, starts a session) when needed, then\n" +
			"wires the always-on integration (AGENTS.md + MCP + instructions) for the agents\n" +
			"installed on this machine. Use --only to wire a single agent, or --no-agents for\n" +
			"the session layer only.\n" +
			"Idempotent — safe to re-run. Run once per project.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := cwd()
			bin := selfPath()

			app, err := bootstrap.Open(".")
			if err != nil {
				app, err = bootstrap.Init(dir, "cli")
				if err != nil {
					return err
				}
				fmt.Printf("✓ project initialized\n")
			} else {
				fmt.Printf("✓ project already initialized\n")
			}

			if regErr := registerProject(app.Root, app.Cfg.Project.Name); regErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not register project: %v\n", regErr)
			}

			if noAgents {
				return nil
			}

			path, err := agent.EnsureAGENTSMD(dir)
			if err != nil {
				return err
			}
			if path != "" {
				fmt.Printf("✓ wrote universal instructions to %s\n", path)
			} else {
				fmt.Printf("✓ AGENTS.md already has Agent Session instructions\n")
			}

			if only == "" {
				// wire only the agents installed on this machine
				wired := 0
				for _, a := range agent.DetectInstalled() {
					if !a.Present {
						continue
					}
					installAgent(dir, bin, a.Name)
					wired++
				}
				if wired == 0 {
					yellow("- no agent CLIs detected (claude, opencode, codex, cursor)\n")
					yellow("  AGENTS.md still covers any agent that reads it. Re-run `init --only <agent>` after installing one.\n")
				}
			} else {
				installAgent(dir, bin, only)
			}
			fmt.Printf("done. agent-session is now active in this project.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Only wire one agent: claude | opencode | codex | cursor | cline")
	cmd.Flags().BoolVar(&noAgents, "no-agents", false, "Session layer only, skip agent wiring")
	return cmd
}

func installAgent(dir, bin, name string) {
	switch name {
	case "claude":
		installClaude(dir, bin)
	case "opencode":
		installOpenCode(dir, bin)
	case "codex":
		installCodex(bin)
	case "cursor":
		installCursor(dir, bin)
	case "cline":
		installCline(dir, bin)
	default:
		red("✗ unknown agent %q\n", name)
	}
}
