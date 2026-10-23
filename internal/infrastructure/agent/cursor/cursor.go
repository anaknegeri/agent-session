package cursor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "cursor" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	cursorDir := filepath.Join(a.projectRoot, ".cursor")
	_, err := os.Stat(cursorDir)
	return err == nil, nil
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	cursorDir := filepath.Join(a.projectRoot, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return fmt.Errorf("create .cursor dir: %w", err)
	}

	if err := a.writeMCPConfig(cursorDir, mcpCommand); err != nil {
		return err
	}
	return a.writeRules()
}

func (a *Adapter) writeMCPConfig(cursorDir, mcpCommand string) error {
	path := filepath.Join(cursorDir, "mcp.json")
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	entry := agent.Section(agent.Section(config, "mcpServers"), "agent-session")
	entry["command"] = mcpCommand
	entry["args"] = []any{"mcp"}
	agent.Section(entry, "env")["AGENT_SESSION_AGENT"] = "cursor"
	return agent.WriteJSONConfig(path, config)
}

// rulesContent is a whole-file resource: the marker goes in an HTML comment
// after the YAML frontmatter, the way commands.Command.Render does it, so
// Cursor still parses `---` as the first line.
const rulesContent = `---
description: Agent Session — universal session & handoff layer for AI coding agents
alwaysApply: true
---
<!-- ` + agent.ManagedMarker + ` -->

` + agentInstructions

const agentInstructions = `This project uses Agent Session (agent-session) as its session layer. Always follow this workflow:

1. When a session starts, FIRST call the agent-session MCP tools in order:
   - session.get — find the current session
   - context.get — load the current context
   Continue the existing task; do not start from scratch.
   The context summary is a bounded preview for token savings — call
   context.get depth=full whenever you need complete decisions, blockers,
   changed files, or the full event list.
2. Record work as you go:
   - task.create / task.update — track the current task
   - decision.create — record architectural decisions with a reason
   - blocker.create — record blockers
   - event.append — record test results (test.failed / test.passed) and file.changed
3. Before finishing (Stop / session end), create a checkpoint:
   - session.checkpoint — include the next_action so the next agent can continue.
`

// RulesPath is the always-applied rule file Cursor loads from a project.
// Exported so anything that inspects our wiring resolves the same path this
// adapter writes and removes.
func RulesPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".cursor", "rules", "agent-session.mdc")
}

// writeRules installs the rule through the managed path. It used to skip any
// non-empty file, which meant a corrected rule could never ship to anyone who
// had already run init — and the rule is the only thing that makes Cursor follow
// the workflow, so a wrong one stayed wrong forever.
func (a *Adapter) writeRules() error {
	_, err := agent.WriteManaged(RulesPath(a.projectRoot), rulesContent)
	return err
}

func (a *Adapter) Install(ctx context.Context) error { return nil }

// Uninstall deregisters the MCP server and removes the rule.
//
// Leaving the rule behind kept Cursor injecting alwaysApply instructions to call
// agent-session MCP tools that this same call deregistered, so every session in
// the project started by trying to use tools that no longer exist.
func (a *Adapter) Uninstall(ctx context.Context) error {
	if _, err := agent.RemoveManaged(RulesPath(a.projectRoot)); err != nil {
		return err
	}
	// The rule lives in a directory of its own only when the user has no other
	// rules, so remove it just when empty.
	_ = os.Remove(filepath.Join(a.projectRoot, ".cursor", "rules"))

	path := filepath.Join(a.projectRoot, ".cursor", "mcp.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}
	delete(servers, "agent-session")
	if len(servers) == 0 {
		delete(config, "mcpServers")
	}
	if len(config) == 0 {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return agent.WriteJSONConfig(path, config)
}

var _ agent.Adapter = (*Adapter)(nil)
