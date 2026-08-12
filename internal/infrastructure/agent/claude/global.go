package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// EnsureGlobalHooks merges the guarded SessionStart/Stop/PreCompact hooks into
// ~/.claude/settings.json without touching unrelated settings. Idempotent —
// re-running does not duplicate entries.
func EnsureGlobalHooks(home string) error {
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	settings := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, command := range guardedHookCommands() {
		if hasAgentSessionHook(hooks, event) {
			continue
		}
		entries, _ := hooks[event].([]any)
		hooks[event] = append(entries, hookEntry(command))
	}
	settings["hooks"] = hooks

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
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

// RemoveGlobalHooks strips only the agent-session hook entries from
// ~/.claude/settings.json, leaving any other hooks or settings untouched.
// No-op if the file or the entries don't exist.
func RemoveGlobalHooks(home string) error {
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
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
	} else {
		settings["hooks"] = hooks
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// globalRuleSection is scoped with a .agent/ guard so it is inert in every
// project that doesn't use agent-session, unlike an unconditional rule.
const globalRuleSection = "## Agent Session"

var globalRule = globalRuleSection + `

If this project contains a ` + "`.agent/`" + ` directory (created by ` + "`agent-session init`" + `),
it uses Agent Session as its session layer. SessionStart/Stop hooks already keep
session state loaded and checkpointed automatically. As you work, still record
state via the agent-session MCP tools: ` + "`task.create`/`task.update`" + `,
` + "`decision.create`" + `, ` + "`blocker.create`" + `, and ` + "`event.append`" + `
for test results. Ignore this note entirely in projects without a ` + "`.agent/`" + ` directory.
`

// EnsureGlobalRule idempotently appends the Agent Session rule to
// ~/.claude/CLAUDE.md. Returns the path written, or "" when already present.
func EnsureGlobalRule(home string) (string, error) {
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		content = nil
	}
	if strings.Contains(string(content), globalRuleSection) {
		return "", nil
	}

	header := ""
	if len(content) > 0 {
		header = "\n"
	}
	updated := string(content) + header + globalRule
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create .claude dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// RemoveGlobalRule strips the Agent Session section from ~/.claude/CLAUDE.md,
// leaving any other content untouched. No-op if the file or section is absent.
func RemoveGlobalRule(home string) error {
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !strings.Contains(string(content), globalRuleSection) {
		return nil
	}

	updated := strings.Replace(string(content), "\n"+globalRule, "", 1)
	if updated == string(content) {
		updated = strings.Replace(string(content), globalRule, "", 1)
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}
