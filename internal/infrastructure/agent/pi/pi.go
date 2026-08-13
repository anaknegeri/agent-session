// Package pi wires Agent Session into pi (@earendil-works/pi-coding-agent).
//
// pi is the one supported agent with no MCP client, and that is a stated design
// choice of the tool, not a gap waiting to be filled: "No MCP. Build CLI tools
// with READMEs (see Skills), or build an extension that adds MCP support." So
// this adapter registers no MCP server. It installs three things instead:
//
//   - extensions/agent-session.ts — the pi equivalent of the SessionStart / Stop /
//     PreCompact hooks: it shells out to the agent-session CLI on session_start,
//     session_before_compact and session_shutdown
//   - skills/agent-session/SKILL.md — tells the model how to record work, in CLI
//     calls rather than tool names
//   - prompts/*.md — the /agent-session slash commands, likewise CLI-flavoured
//
// The directory layout is asymmetric, and pi's own docs are the reason:
// user scope lives under ~/.pi/agent/, project scope under <project>/.pi/.
//
// Project-local extensions, skills and prompts load only after the user has
// trusted the project in pi. agent-session never writes that trust record — the
// approval gate is what stops an installer from wiring shell commands into
// someone's agent — so project-scope wiring is inert until the user approves it.
package pi

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

// managedMarker matches the marker in both embedded files, so re-running setup
// updates our files and never touches one the user wrote or heavily edited.
const managedMarker = "agent-session:managed"

// UserRoot is pi's user-scope resource root.
func UserRoot(home string) string { return filepath.Join(home, ".pi", "agent") }

// ProjectRoot is pi's project-scope resource root.
func ProjectRoot(projectDir string) string { return filepath.Join(projectDir, ".pi") }

func extensionPath(root string) string {
	return filepath.Join(root, "extensions", "agent-session.ts")
}

func skillDir(root string) string { return filepath.Join(root, "skills", "agent-session") }

// PromptsDir is where pi discovers slash-command templates.
func PromptsDir(root string) string { return filepath.Join(root, "prompts") }

// renderExtension bakes the binary path into the extension source.
func renderExtension(bin string) string {
	if bin == "" {
		bin = "agent-session"
	}
	return strings.Replace(extensionTS, binPlaceholder, strconv.Quote(bin), 1)
}

// EnsureExtension writes the lifecycle extension under root.
func EnsureExtension(root, bin string) error {
	return writeManaged(extensionPath(root), renderExtension(bin))
}

// EnsureSkill writes the CLI skill under root.
func EnsureSkill(root string) error {
	return writeManaged(filepath.Join(skillDir(root), "SKILL.md"), skillMD)
}

// EnsurePrompts writes the pi slash commands under root.
func EnsurePrompts(root string) error {
	_, err := commands.InstallSet(PromptsDir(root), Commands())
	return err
}

// EnsureResources installs everything pi needs at the given scope root. It is
// idempotent and never overwrites a file that is not ours.
func EnsureResources(root, bin string) error {
	if err := EnsureExtension(root, bin); err != nil {
		return err
	}
	if err := EnsureSkill(root); err != nil {
		return err
	}
	return EnsurePrompts(root)
}

// RemoveResources reverses EnsureResources, leaving files we do not own and any
// other pi configuration untouched.
func RemoveResources(root string) error {
	if err := removeManaged(extensionPath(root)); err != nil {
		return err
	}
	if err := removeManaged(filepath.Join(skillDir(root), "SKILL.md")); err != nil {
		return err
	}
	// The skill lives in a directory of its own, so removing it leaves an empty
	// one behind unless we clean up. Only if it is empty: a user may have put
	// something else in there.
	_ = os.Remove(skillDir(root))
	if _, err := commands.UninstallSet(PromptsDir(root), Commands()); err != nil {
		return err
	}
	return nil
}

func writeManaged(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !strings.Contains(string(existing), managedMarker) {
			return nil
		}
		if string(existing) == content {
			return nil
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func removeManaged(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if !strings.Contains(string(data), managedMarker) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// Adapter wires pi at project scope.
type Adapter struct {
	projectRoot string
}

func NewAdapter(projectRoot string) *Adapter {
	return &Adapter{projectRoot: projectRoot}
}

func (a *Adapter) Name() string { return "pi" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	if _, err := os.Stat(ProjectRoot(a.projectRoot)); err == nil {
		return true, nil
	}
	return false, nil
}

// Configure writes the lifecycle extension. mcpCommand is the agent-session
// binary path; despite the interface name nothing MCP is registered, because pi
// has no MCP client to register with.
func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	return EnsureExtension(ProjectRoot(a.projectRoot), mcpCommand)
}

// Install writes the skill and the slash commands.
func (a *Adapter) Install(ctx context.Context) error {
	root := ProjectRoot(a.projectRoot)
	if err := EnsureSkill(root); err != nil {
		return err
	}
	return EnsurePrompts(root)
}

func (a *Adapter) Uninstall(ctx context.Context) error {
	return RemoveResources(ProjectRoot(a.projectRoot))
}

var _ agent.Adapter = (*Adapter)(nil)
