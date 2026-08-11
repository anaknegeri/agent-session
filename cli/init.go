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
		Short:   "One-command setup: init the project and wire every agent (like git init)",
		Long: "Initializes the project (creates .agent/, starts a session) when needed, then\n" +
			"wires the always-on integration for every agent (AGENTS.md, claude, opencode, codex).\n" +
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

			if only == "" || only == "claude" {
				installClaude(dir, bin)
			}
			if only == "" || only == "opencode" {
				installOpenCode(dir, bin)
			}
		if only == "" || only == "codex" {
			installCodex(bin)
		}
		if only == "" || only == "cursor" {
			installCursor(dir, bin)
		}
		if only == "" || only == "cline" {
			installCline(dir, bin)
		}
		fmt.Printf("done. agent-session is now active in this project.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Only wire one agent: claude | opencode | codex")
	cmd.Flags().BoolVar(&noAgents, "no-agents", false, "Session layer only, skip agent wiring")
	return cmd
}
