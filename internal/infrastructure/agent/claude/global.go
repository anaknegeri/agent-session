package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

// guardedHookCommands maps Claude Code hook events to shell commands that
// only act inside an agent-session project and never fail the hook (|| true),
// so they are silent no-ops in every other project.
//
// agent-session has no --dir flag — it resolves the project root from the
// process's own working directory (os.Getwd()), which a hook subprocess is
// not guaranteed to inherit. Anchoring on $CLAUDE_PROJECT_DIR (the project
// root Claude Code was started in, always set for hook subprocesses) makes
// both the .agent existence check and the command itself independent of
// whatever cwd the hook actually runs with.
func guardedHookCommands() map[string]string {
	const cdProject = `cd "$CLAUDE_PROJECT_DIR" 2>/dev/null && `
	return map[string]string{
		"SessionStart": cdProject + "[ -d .agent ] && agent-session resume --agent claude || true",
		"Stop":         cdProject + "[ -d .agent ] && agent-session checkpoint --label auto || true",
		"PreCompact":   cdProject + "[ -d .agent ] && agent-session checkpoint --label precompact || true",
	}
}

func hookEntry(command string) map[string]any {
	return map[string]any{
		"matcher": "*",
		"hooks": []map[string]any{
			{"type": "command", "command": command},
		},
	}
}

// HasAgentSessionHooks reports whether the given settings map (the parsed
// contents of ~/.claude/settings.json) contains the agent-session guarded hooks.
// Used by `agent-session doctor` to verify user-scope wiring.
func HasAgentSessionHooks(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	for event := range guardedHookCommands() {
		if !hasAgentSessionHook(hooks, event) {
			return false
		}
	}
	return true
}

// GlobalSettingsPath is the user-scope settings file Claude Code merges into
// every project.
func GlobalSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

// GlobalRulePath is the user-scope memory file Claude Code always reads.
func GlobalRulePath(home string) string {
	return filepath.Join(home, ".claude", "CLAUDE.md")
}

// UserConfigPath is where Claude Code keeps its user-scope MCP registrations —
// a different file from settings.json, and the one `claude mcp add --scope user`
// writes.
func UserConfigPath(home string) string { return filepath.Join(home, ".claude.json") }

// UserMCPCommand returns the command Claude Code has registered for the
// agent-session MCP server at user scope, or "" when there is none.
//
// `claude mcp add` refuses to touch an entry that already exists, so setup needs
// to know the registered path to notice a stale one: after the binary moves — a
// self-update, a switch from Homebrew to ~/.local/bin — an unchanged entry points
// at a file that is gone, and Claude Code loses every session tool with no error
// anywhere. Every other adapter rewrites `command` on re-run; this makes the
// Claude path able to do the same.
func UserMCPCommand(home string) (string, error) {
	config, err := agent.ReadJSONConfig(UserConfigPath(home))
	if err != nil {
		return "", err
	}
	servers, _ := config["mcpServers"].(map[string]any)
	entry, _ := servers["agent-session"].(map[string]any)
	command, _ := entry["command"].(string)
	return command, nil
}

// EnsureGlobalHooks merges the guarded hooks into ~/.claude/settings.json.
func EnsureGlobalHooks(home string) error { return EnsureHooks(GlobalSettingsPath(home)) }

// RemoveGlobalHooks strips them from ~/.claude/settings.json.
func RemoveGlobalHooks(home string) error { return RemoveHooks(GlobalSettingsPath(home)) }

// EnsureGlobalRule appends the guarded rule to ~/.claude/CLAUDE.md.
func EnsureGlobalRule(home string) (string, error) {
	return EnsureRule(GlobalRulePath(home), GlobalRule)
}

// RemoveGlobalRule strips the guarded rule from ~/.claude/CLAUDE.md.
func RemoveGlobalRule(home string) error { return RemoveRule(GlobalRulePath(home), GlobalRule) }

// EnsureHooks merges the guarded SessionStart/Stop/PreCompact hooks into a Claude
// Code settings file without touching unrelated settings. Idempotent —
// re-running does not duplicate entries. Used for both scopes: the guards are
// what make the same commands correct in a user-scope file and in a project one,
// since a hook subprocess does not inherit the project's working directory.
func EnsureHooks(path string) error {
	settings, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}

	hooks := agent.Section(settings, "hooks")
	for event, command := range guardedHookCommands() {
		if hasAgentSessionHook(hooks, event) {
			continue
		}
		entries, _ := hooks[event].([]any)
		hooks[event] = append(entries, hookEntry(command))
	}
	return agent.WriteJSONConfig(path, settings)
}

func hasAgentSessionHook(hooks map[string]any, event string) bool {
	entries, _ := hooks[event].([]any)
	for _, e := range entries {
		if isAgentSessionEntry(e) {
			return true
		}
	}
	return false
}

func isAgentSessionEntry(e any) bool {
	entry, ok := e.(map[string]any)
	if !ok {
		return false
	}
	inner, _ := entry["hooks"].([]any)
	for _, h := range inner {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, _ := hm["command"].(string); strings.Contains(cmd, "agent-session") {
			return true
		}
	}
	return false
}

// RemoveHooks strips only the agent-session hook entries from a Claude Code
// settings file, leaving any other hooks or settings untouched. No-op if the file
// or the entries don't exist.
func RemoveHooks(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	settings, err := agent.ReadJSONConfig(path)
	if err != nil {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}

	changed := false
	for event := range guardedHookCommands() {
		entries, _ := hooks[event].([]any)
		kept := entries[:0]
		for _, e := range entries {
			if isAgentSessionEntry(e) {
				changed = true
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !changed {
		return nil
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	// A settings file that held nothing but our hooks is ours to remove; anything
	// else in it belongs to the user and stays.
	if len(settings) == 0 {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return agent.WriteJSONConfig(path, settings)
}

// ruleSection is the heading both rules share, and the marker that makes
// appending them idempotent and removal surgical.
const ruleSection = "## Agent Session"

// GlobalRule is scoped with a .agent/ guard so it is inert in every project that
// doesn't use agent-session, unlike an unconditional rule.
var GlobalRule = ruleSection + `

If this project contains a ` + "`.agent/`" + ` directory (created by ` + "`agent-session init`" + `),
it uses Agent Session as its session layer. SessionStart/Stop hooks already keep
session state loaded and checkpointed automatically. As you work, still record
state via the agent-session MCP tools: ` + "`task.create`/`task.update`" + `,
` + "`decision.create`" + `, ` + "`blocker.create`" + `, and ` + "`event.append`" + `
for test results. Ignore this note entirely in projects without a ` + "`.agent/`" + ` directory.
`

// ProjectRule needs no guard: it is only ever written into the project that was
// initialized, so it can state the workflow directly.
var ProjectRule = ruleSection + `

This project uses Agent Session (agent-session) as its session layer.

- At the start of a session, FIRST call the agent-session MCP tools in order:
  session.get, then context.get. Continue the existing task; do not start from scratch.
- The context summary is a bounded preview (token savings). Call
  ` + "`context.get depth=full`" + ` whenever you need complete decisions, blockers,
  changed files, or events — never act on incomplete info when detail is one call away.
- Record work as you go: task.create / task.update, decision.create, blocker.create,
  and event.append for test results (test.failed / test.passed).
- Before finishing (Stop), create a checkpoint with session.checkpoint including next_action.
- To keep context small, summarize before finishing: call context.summarize, then
  store the summary with memory.put (kind=project_knowledge).
`

// EnsureRule idempotently appends rule to a CLAUDE.md, preserving whatever the
// user already wrote there. Returns the path written, or "" when already present.
//
// Appending rather than writing is the whole contract: a CLAUDE.md is the user's
// project memory, and setup has no business replacing it.
func EnsureRule(path, rule string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		content = nil
	}
	if strings.Contains(string(content), ruleSection) {
		return "", nil
	}

	header := ""
	if len(content) > 0 {
		header = "\n"
	}
	updated := string(content) + header + rule
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// RemoveRule strips the Agent Session section from a CLAUDE.md, leaving any other
// content untouched. A file that held nothing else is removed. No-op if the file
// or the section is absent.
func RemoveRule(path, rule string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !strings.Contains(string(content), ruleSection) {
		return nil
	}

	updated := strings.Replace(string(content), "\n"+rule, "", 1)
	if updated == string(content) {
		updated = strings.Replace(string(content), rule, "", 1)
	}
	if strings.TrimSpace(updated) == "" {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
