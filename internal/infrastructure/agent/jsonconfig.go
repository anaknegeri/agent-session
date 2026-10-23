package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadJSONConfig loads a JSON config file that belongs to another tool.
//
// Absent or blank file: an empty config, so the caller can create one. Present
// but unparseable — or unreadable for any other reason — is an error, never an
// empty config. That distinction is the entire point of this helper: these files
// hold the user's own MCP servers, keybinds and editor settings, and a
// "start over from {}" fallback silently converts a typo, a JSONC comment or a
// permission problem into data loss the user cannot get back.
func ReadJSONConfig(path string) (map[string]any, error) {
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
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w (fix or move the file, then re-run)", path, err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

// WriteJSONConfig writes config to path, creating parent directories. The
// replacement is atomic (see WriteFileAtomic): these files hold the user's own
// MCP servers and editor settings, and a truncated one is exactly the
// unrecoverable state ReadJSONConfig refuses to paper over.
//
// HTML escaping is off: these files carry shell commands and URLs, and a hook
// command rendered as `agent-session resume \u0026\u0026 …` is valid JSON that
// reads like corruption to the person who opens it next.
func WriteJSONConfig(path string, config map[string]any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(config); err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o644)
}

// Section returns config[key] as a map for in-place editing, creating an empty
// one when the key is absent (or holds something that is not an object), and
// storing it back under key.
func Section(config map[string]any, key string) map[string]any {
	section, _ := config[key].(map[string]any)
	if section == nil {
		section = map[string]any{}
	}
	config[key] = section
	return section
}

// ManagedByKey and ManagedByValue mark a JSON object agent-session added to a
// config file it shares with the user and with other tools — a hook group in
// ~/.claude/settings.json or ~/.codex/hooks.json.
//
// Ownership used to be "the command mentions agent-session", which also claims
// a user's own my-agent-session-wrapper.sh hook and deletes it on uninstall.
// An explicit key on the entry we wrote cannot be guessed wrong.
const (
	ManagedByKey   = "managedBy"
	ManagedByValue = "agent-session"
)

// OwnsJSONEntry reports whether entry carries our ownership key.
func OwnsJSONEntry(entry map[string]any) bool {
	value, _ := entry[ManagedByKey].(string)
	return value == ManagedByValue
}

// legacyHookInvocations are the agent-session command shapes every version has
// written into a hook. Versions before ManagedByKey marked nothing, so removal
// still has to recognise them — but by what we actually generate, not by the
// bare product name, which a user's own wrapper script also contains.
var legacyHookInvocations = []string{
	"agent-session resume",
	"agent-session checkpoint",
}

// OwnsLegacyHookCommand reports whether cmd is a hook command an agent-session
// release before ManagedByKey wrote. Use it only when removing: new entries get
// the explicit key.
func OwnsLegacyHookCommand(cmd string) bool {
	for _, invocation := range legacyHookInvocations {
		if strings.Contains(cmd, invocation) {
			return true
		}
	}
	return false
}
