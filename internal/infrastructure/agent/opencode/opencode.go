package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

// SystemInstructions is the always-on rule for a project already known to use
// agent-session (project-level opencode.json only ever applies to that project).
const SystemInstructions = "This project uses Agent Session. At the start of a session, FIRST call the agent-session MCP tools in order: session.get then context.get, and continue the existing task. The context summary is a bounded preview — call context.get depth=full when you need complete decisions, blockers, changed files, or events. Record work with task.create/task.update, decision.create, blocker.create, and event.append for test results. Before finishing, create a checkpoint with session.checkpoint including next_action, and summarize with context.summarize then memory.put (kind=project_knowledge)."

// Adapter configures OpenCode via project-level opencode.json.
type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "opencode" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	paths := []string{
		filepath.Join(a.projectRoot, "opencode.json"),
		filepath.Join(a.projectRoot, "opencode.jsonc"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true, nil
		}
	}
	return false, nil
}

// ConfigPath is the project config OpenCode reads. Detect accepts the .jsonc form
// too, and when that is the file the project standardized on, writing a second
// .json beside it would leave the wiring in a file the user does not maintain.
func (a *Adapter) ConfigPath() string {
	jsonc := filepath.Join(a.projectRoot, "opencode.jsonc")
	if _, err := os.Stat(jsonc); err == nil {
		return jsonc
	}
	return filepath.Join(a.projectRoot, "opencode.json")
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	path := a.ConfigPath()
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}

	entry := agent.Section(agent.Section(config, "mcp"), "agent-session")
	entry["type"] = "local"
	entry["command"] = []any{mcpCommand, "mcp"}
	entry["enabled"] = true
	agent.Section(entry, "environment")["AGENT_SESSION_AGENT"] = "opencode"

	// Appended, never assigned: agent.instructions.system is where the user keeps
	// their own always-on prompt, and this is the only copy of it.
	MergeInstructions(config, SystemInstructions)

	return agent.WriteJSONConfig(path, config)
}

func (a *Adapter) Install(ctx context.Context) error {
	return nil
}

// Uninstall removes our MCP entry and our instruction note, leaving every other
// server, setting and custom instruction in place.
func (a *Adapter) Uninstall(ctx context.Context) error {
	path := a.ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	servers, _ := config["mcp"].(map[string]any)
	if servers != nil {
		delete(servers, "agent-session")
		if len(servers) == 0 {
			delete(config, "mcp")
		}
	}
	RemoveInstructions(config, SystemInstructions)
	if len(config) == 0 {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return agent.WriteJSONConfig(path, config)
}

var _ agent.Adapter = (*Adapter)(nil)
