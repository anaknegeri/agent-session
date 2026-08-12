package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

func newInitCmd() *cobra.Command {
	var only string
	var noAgents bool
	var projectScope bool
	cmd := &cobra.Command{
		Use:     "init",
		Aliases: []string{"setup"},
		Short:   "One-command setup: init the project and wire agents at user scope ",
		Long: "Initializes the project (creates .agent/, starts a session) when needed.\n" +
			"By default it wires the agent-session MCP server ONCE at user scope \n" +
			"for the agents installed on this machine, so projects stay clean. Use --project to\n" +
			"write per-project config instead, --only to wire a single agent, or --no-agents for\n" +
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
				// default: wire at user scope  for installed agents
				wired := 0
				for _, a := range agent.DetectInstalled() {
					if !a.Present {
						continue
					}
					if projectScope {
						installAgent(dir, bin, a.Name)
					} else {
						installAgentGlobal(bin, a.Name)
					}
					wired++
				}
				if wired == 0 {
					yellow("- no agent CLIs detected (claude, opencode, codex, cursor)\n")
					yellow("  AGENTS.md still covers any agent that reads it. Re-run `init --only <agent>` after installing one.\n")
				}
			} else if projectScope {
				installAgent(dir, bin, only)
			} else {
				installAgentGlobal(bin, only)
			}
			fmt.Printf("done. agent-session is now active in this project.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Only wire one agent: claude | opencode | codex | cursor | cline")
	cmd.Flags().BoolVar(&noAgents, "no-agents", false, "Session layer only, skip agent wiring")
	cmd.Flags().BoolVar(&projectScope, "project", false, "Write per-project agent config instead of user scope")
	return cmd
}

// installAgentGlobal wires the MCP server at user scope  so no
// per-project config files are created. Fallbacks write the global config file.
func installAgentGlobal(bin, name string) {
	switch name {
	case "claude":
		cmd := exec.Command("claude", "mcp", "add", "--scope", "user", "agent-session", "--", bin, "mcp")
		if out, err := cmd.CombinedOutput(); err != nil {
			red("✗ claude (user scope): %v: %s\n", err, strings.TrimSpace(string(out)))
			return
		}
		green("✓ claude: registered at user scope (always available)\n")
	case "opencode":
		if err := installOpenCodeGlobal(bin); err != nil {
			red("✗ opencode: %v\n", err)
			return
		}
		green("✓ opencode: registered in ~/.config/opencode/opencode.json\n")
	case "codex":
		installCodex(bin)
	case "cursor":
		if err := installCursorGlobal(bin); err != nil {
			red("✗ cursor: %v\n", err)
			return
		}
		green("✓ cursor: registered in ~/.cursor/mcp.json\n")
	case "cline":
		red("✗ cline has no user scope; use `init --only cline` for per-project wiring\n")
	default:
		red("✗ unknown agent %q\n", name)
	}
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
