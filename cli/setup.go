package cli

import (
	"os/exec"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/claude"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cline"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/codex"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/cursor"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/opencode"
)

// installClaude wires Claude Code via project .mcp.json + CLAUDE.md + hooks.
func installClaude(dir, bin string) {
	a := claude.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ claude: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		red("✗ claude: %v\n", err)
		return
	}
	green("✓ claude: .mcp.json + .claude/CLAUDE.md + hooks (auto resume/checkpoint)\n")
}

func installOpenCode(dir, bin string) {
	a := opencode.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ opencode: %v\n", err)
		return
	}
	if err := a.Install(ctx()); err != nil {
		red("✗ opencode: %v\n", err)
		return
	}
	green("✓ opencode: opencode.json (mcp + always-on instructions)\n")
}

func installCodex(bin string) {
	if _, err := exec.LookPath("codex"); err != nil {
		yellow("- codex: CLI not found, skipping (AGENTS.md covers it)\n")
		return
	}
	a := codex.NewAdapter()
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ codex: %v\n", err)
		return
	}
	green("✓ codex: mcp_servers.agent-session registered\n")
}

func installCursor(dir, bin string) {
	a := cursor.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ cursor: %v\n", err)
		return
	}
	green("✓ cursor: .cursor/mcp.json + .cursor/rules/agent-session.mdc\n")
}

func installCline(dir, bin string) {
	a := cline.NewAdapter(dir)
	if err := a.Configure(ctx(), bin); err != nil {
		red("✗ cline: %v\n", err)
		return
	}
	green("✓ cline: .clinerules + .vscode/settings.json (cline.mcpServers)\n")
}
