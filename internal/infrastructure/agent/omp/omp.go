// Package omp wires Agent Session into omp (oh-my-pi, @oh-my-pi/pi-coding-agent).
//
// omp is a pi derivative and shares its extension API, but it cannot share the
// pi adapter, for two reasons that each break the wiring on their own:
//
//   - Roots differ. omp discovers native resources under <project>/.omp and
//     ~/.omp/agent; .pi/extensions is explicitly not a native root for omp, so
//     everything the pi adapter installs is invisible to it.
//   - omp ships an MCP client, which pi deliberately does not. So the skill and
//     the slash commands here are the universal MCP-tool set, not pi's
//     CLI-flavoured rewrite of it, and this adapter registers the MCP server.
//
// Per scope root, `EnsureResources` installs:
//
//	mcp.json                       agent-session MCP server (stdio)
//	extensions/agent-session.ts    resume on session_start, checkpoint on
//	                               compaction and shutdown — the omp equivalent
//	                               of Claude Code's SessionStart/Stop hooks
//	skills/agent-session/SKILL.md  how to read and record state through MCP
//	commands/*.md                  the universal /agent-session slash commands
//
// The MCP server alone gives the model the tools; the extension is what makes
// continuity independent of whether the model remembers to call them.
package omp

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
	"github.com/anaknegeri/agent-session/internal/infrastructure/agent/commands"
)

//go:embed extension.ts
var extensionTS string

//go:embed SKILL.md
var skillMD string

// binPlaceholder is replaced with a quoted TypeScript string literal holding the
// absolute path of the agent-session binary.
const binPlaceholder = "__AGENT_SESSION_BIN__"

const managedMarker = agent.ManagedMarker

// serverName is the MCP server key. It matches the other adapters, so a project
// that wires several agents exposes the same tool names in each.
const serverName = "agent-session"

// mcpSchema is the schema omp publishes for its own MCP config. Writing it makes
// the file validate in an editor; omp itself does not require it.
const mcpSchema = "https://raw.githubusercontent.com/can1357/oh-my-pi/main/packages/coding-agent/src/config/mcp-schema.json"

// UserRoot is omp's user-scope resource root: the agent directory omp itself
// resolves. omp reads the default profile's ~/.omp/agent unless an environment
// override is active, and wiring the wrong one of those installs files omp never
// looks at, so the same precedence is honored here:
//
//	PI_CODING_AGENT_DIR              explicit agent directory, wins outright
//	OMP_PROFILE / PI_PROFILE = x     ~/.omp/profiles/x/agent
//	otherwise                        ~/.omp/agent
//
// A profile passed only as `omp --profile x` cannot be seen from here; that case
// needs project-scope wiring or an exported OMP_PROFILE.
func UserRoot(home string) string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return dir
	}
	profile := strings.TrimSpace(os.Getenv("OMP_PROFILE"))
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv("PI_PROFILE"))
	}
	if profile != "" && profile != "default" {
		return filepath.Join(home, ".omp", "profiles", profile, "agent")
	}
	return filepath.Join(home, ".omp", "agent")
}

// ProjectRoot is omp's project-scope resource root.
func ProjectRoot(projectDir string) string { return filepath.Join(projectDir, ".omp") }

// ExtensionPath is the lifecycle extension omp loads from a scope root. Exported
// so `doctor` checks the same path setup writes.
func ExtensionPath(root string) string {
	return filepath.Join(root, "extensions", "agent-session.ts")
}

func skillDir(root string) string { return filepath.Join(root, "skills", "agent-session") }

// CommandsDir is where omp discovers slash-command markdown.
func CommandsDir(root string) string { return filepath.Join(root, "commands") }

// MCPPath is the config file omp reads MCP servers from at this scope.
func MCPPath(root string) string { return filepath.Join(root, "mcp.json") }

// renderExtension bakes the binary path into the extension source.
func renderExtension(bin string) string {
	if bin == "" {
		bin = "agent-session"
	}
	return strings.Replace(extensionTS, binPlaceholder, strconv.Quote(bin), 1)
}

// EnsureExtension writes the lifecycle extension under root.
func EnsureExtension(root, bin string) error {
	return agent.WriteManaged(ExtensionPath(root), renderExtension(bin))
}

// EnsureSkill writes the MCP skill under root.
func EnsureSkill(root string) error {
	return agent.WriteManaged(filepath.Join(skillDir(root), "SKILL.md"), skillMD)
}

// EnsureCommands writes the universal slash commands under root.
func EnsureCommands(root string) error {
	_, err := commands.Install(CommandsDir(root))
	return err
}

// EnsureMCP registers the agent-session MCP server in root/mcp.json, preserving
// every other server and key in the file — and, on an entry that already exists,
// every key on that entry we do not own. `init` is documented as safe to re-run,
// so it must not silently re-enable a server the user disabled with `/mcp disable`
// or drop a `timeout`, `requestIdFormat`, `auth` block or extra `env` key they set.
func EnsureMCP(root, bin string) error {
	if bin == "" {
		bin = "agent-session"
	}
	path := MCPPath(root)
	config, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	if _, ok := config["$schema"]; !ok {
		config["$schema"] = mcpSchema
	}
	servers := agent.Section(config, "mcpServers")
	entry := agent.Section(servers, serverName)
	entry["type"] = "stdio"
	entry["command"] = bin
	entry["args"] = []any{"mcp"}
	agent.Section(entry, "env")["AGENT_SESSION_AGENT"] = "omp"
	return agent.WriteJSONConfig(path, config)
}

// RemoveMCP deregisters the agent-session MCP server, leaving any other server
// definition in place. A file that ends up holding nothing but the schema line is
// deleted rather than left as litter.
func RemoveMCP(root string) error {
	path := MCPPath(root)
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
	delete(servers, serverName)
	if len(servers) == 0 {
		delete(config, "mcpServers")
	}
	if len(config) == 0 || (len(config) == 1 && config["$schema"] != nil) {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return agent.WriteJSONConfig(path, config)
}

// EnsureResources installs everything omp needs at the given scope root. It is
// idempotent and never overwrites a file that is not ours.
func EnsureResources(root, bin string) error {
	if err := EnsureMCP(root, bin); err != nil {
		return err
	}
	if err := EnsureExtension(root, bin); err != nil {
		return err
	}
	if err := EnsureSkill(root); err != nil {
		return err
	}
	return EnsureCommands(root)
}

// RemoveResources reverses EnsureResources, leaving files we do not own and any
// other omp configuration untouched.
//
// A broken mcp.json is reported but never blocks the rest: refusing to rewrite a
// file we cannot parse is right, refusing to remove the files we do own is not —
// that would leave the lifecycle extension resuming and checkpointing after the
// user asked for it to be gone, with no way out but hand-editing JSON.
func RemoveResources(root string) error {
	mcpErr := RemoveMCP(root)
	if err := agent.RemoveManaged(ExtensionPath(root)); err != nil {
		return err
	}
	if err := agent.RemoveManaged(filepath.Join(skillDir(root), "SKILL.md")); err != nil {
		return err
	}
	// The skill has a directory of its own; remove it only when empty, since the
	// user may have added files beside ours.
	_ = os.Remove(skillDir(root))
	if _, err := commands.Uninstall(CommandsDir(root)); err != nil {
		return err
	}
	return mcpErr
}

// Adapter wires omp at project scope.
type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "omp" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	if _, err := os.Stat(ProjectRoot(a.projectRoot)); err == nil {
		return true, nil
	}
	return false, nil
}

// Configure registers the MCP server and installs the lifecycle extension.
func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	root := ProjectRoot(a.projectRoot)
	if err := EnsureMCP(root, mcpCommand); err != nil {
		return err
	}
	return EnsureExtension(root, mcpCommand)
}

// Install writes the skill and the slash commands.
func (a *Adapter) Install(ctx context.Context) error {
	root := ProjectRoot(a.projectRoot)
	if err := EnsureSkill(root); err != nil {
		return err
	}
	return EnsureCommands(root)
}

func (a *Adapter) Uninstall(ctx context.Context) error {
	return RemoveResources(ProjectRoot(a.projectRoot))
}

var _ agent.Adapter = (*Adapter)(nil)
