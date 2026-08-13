package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

// Adapter configures OpenAI Codex through `codex mcp add`, which writes the
// server into the Codex config for us and honours CODEX_HOME.
type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string { return "codex" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	_, err := exec.LookPath("codex")
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	cmd := exec.CommandContext(ctx, "codex", "mcp", "add", "agent-session", "--", mcpCommand, "mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codex mcp add: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// hookCommands are the session-lifecycle hooks agent-session installs for Codex,
// mirroring what Claude Code gets: resume on session start, checkpoint on stop.
//
// Each is guarded on the working directory being an agent-session project and
// ends in `|| true`, so Codex is never blocked by a hook in an unrelated
// repository or by agent-session being absent from PATH.
var hookCommands = map[string]string{
	"SessionStart": `[ -d .agent ] && agent-session resume --agent codex || true`,
	"Stop":         `[ -d .agent ] && agent-session checkpoint --label auto || true`,
}

// hookMarker identifies the commands this adapter owns, so install can be
// idempotent and uninstall can remove ours without touching anyone else's.
const hookMarker = "agent-session"

// Install writes the session lifecycle hooks into $CODEX_HOME/hooks.json.
//
// Codex does support hooks — the same shape Claude Code uses — but agent-session
// only ever registered the MCP server, so a Codex session depended on the model
// choosing to call the tools. With these, resume and checkpoint happen whether it
// does or not.
//
// The file is shared with plugin-provided hooks, so it is merged rather than
// replaced.
//
// Codex discovers these entries (`hooks/list` reports them as source "user",
// enabled) but will not execute them until the user approves them once, which
// records a trusted_hash under [hooks.state] in config.toml. Writing that hash
// here would defeat the gate that stops an installer from wiring shell commands
// into someone's agent, so setup asks the user to approve instead.
func (a *Adapter) Install(ctx context.Context) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create codex config dir: %w", err)
	}
	path := filepath.Join(dir, "hooks.json")

	root, err := readHooks(path)
	if err != nil {
		return err
	}
	hooks := childObject(root, "hooks")

	for event, command := range hookCommands {
		entries := removeOurEntries(hooks[event])
		entries = append(entries, map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command}},
		})
		hooks[event] = entries
	}
	root["hooks"] = hooks

	return writeHooks(path, root)
}

// HooksInstalled reports whether hooks.json carries this adapter's entry for
// every lifecycle event it installs.
//
// It says nothing about whether Codex will run them: that depends on the
// per-hook trusted_hash the user's approval writes under [hooks.state] in
// config.toml, keyed by a source/file/event identifier this adapter does not
// construct and cannot reproduce. Callers should present the approval as a step
// still to be taken, not as a state they have read.
func (a *Adapter) HooksInstalled() (bool, error) {
	dir, err := configDir()
	if err != nil {
		return false, err
	}
	root, err := readHooks(filepath.Join(dir, "hooks.json"))
	if err != nil {
		return false, err
	}
	hooks := childObject(root, "hooks")
	for event := range hookCommands {
		entries, _ := hooks[event].([]any)
		installed := false
		for _, entry := range entries {
			if ownsEntry(entry) {
				installed = true
				break
			}
		}
		if !installed {
			return false, nil
		}
	}
	return true, nil
}

func (a *Adapter) Uninstall(ctx context.Context) error {
	if err := a.uninstallHooks(); err != nil {
		return err
	}
	return a.uninstallMCP()
}

func (a *Adapter) uninstallHooks() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "hooks.json")
	root, err := readHooks(path)
	if err != nil {
		return err
	}
	hooks := childObject(root, "hooks")
	for event := range hookCommands {
		remaining := removeOurEntries(hooks[event])
		if len(remaining) == 0 {
			delete(hooks, event)
			continue
		}
		hooks[event] = remaining
	}
	root["hooks"] = hooks
	return writeHooks(path, root)
}

// readHooks loads hooks.json, treating a missing or empty file as an empty
// configuration so a first install does not need one to exist.
func readHooks(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}

func writeHooks(path string, root map[string]any) error {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("encode hooks: %w", err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func childObject(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	return map[string]any{}
}

// removeOurEntries drops the hook groups this adapter added, keeping every other
// tool's untouched.
func removeOurEntries(value any) []any {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	kept := make([]any, 0, len(entries))
	for _, entry := range entries {
		if ownsEntry(entry) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

func ownsEntry(entry any) bool {
	group, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	inner, ok := group["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range inner {
		hook, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, ok := hook["command"].(string); ok && strings.Contains(cmd, hookMarker) {
			return true
		}
	}
	return false
}

func (a *Adapter) uninstallMCP() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	cfg := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		return nil
	}
	out := removeMCPSection(string(data), "agent-session")
	return os.WriteFile(cfg, []byte(out), 0o644)
}

// ConfigDir returns the directory Codex reads its configuration from: $CODEX_HOME
// when set, ~/.codex otherwise. Exported so callers that inspect the same files
// (doctor) resolve them exactly the way this adapter writes them.
func ConfigDir() (string, error) { return configDir() }

// configDir resolves Codex's config directory the same way Codex does, so an
// uninstall edits the config that `codex mcp add` actually wrote.
func configDir() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func removeMCPSection(toml, server string) string {
	lines := strings.Split(toml, "\n")
	var result []string
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[mcp_servers."+server+"]") {
			skip = true
			continue
		}
		if skip {
			if trimmed == "" || strings.HasPrefix(trimmed, "[") {
				skip = false
			} else {
				continue
			}
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

var _ agent.Adapter = (*Adapter)(nil)
