package cli

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/opencode"
)

func newSetupCmd() *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Make every agent use Agent Session automatically (AGENTS.md + adapters)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := cwd()

			path, err := agent.EnsureAGENTSMD(dir)
			if err != nil {
				return err
			}
			if path != "" {
				fmt.Printf("✓ wrote universal instructions to %s\n", path)
			} else {
				fmt.Printf("✓ AGENTS.md already has Agent Session instructions\n")
			}

			bin := selfPath()
			if only == "" || only == "claude" {
				installClaude(dir, bin)
			}
			if only == "" || only == "opencode" {
				installOpenCode(dir, bin)
			}
			if only == "" || only == "codex" {
				installCodex(bin)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Only wire one agent: claude | opencode | codex")
	return cmd
}

func installClaude(dir, bin string) {
	a := claude.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		fmt.Printf("✗ claude: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		fmt.Printf("✗ claude: %v\n", err)
		return
	}
	fmt.Printf("✓ claude: .mcp.json + .claude/CLAUDE.md + hooks (auto resume/checkpoint)\n")
}

func installOpenCode(dir, bin string) {
	a := opencode.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		fmt.Printf("✗ opencode: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		fmt.Printf("✗ opencode: %v\n", err)
		return
	}
	fmt.Printf("✓ opencode: opencode.json (mcp + always-on instructions)\n")
}

func installCodex(bin string) {
	if _, err := exec.LookPath("codex"); err != nil {
		fmt.Printf("- codex: CLI not found, skipping (AGENTS.md covers it)\n")
		return
	}
	a := codex.NewAdapter()
	if err := a.Configure(ctx(), bin); err != nil {
		fmt.Printf("✗ codex: %v\n", err)
		return
	}
	fmt.Printf("✓ codex: mcp_servers.agent-session registered\n")
}
