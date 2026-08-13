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

// WriteJSONConfig writes config to path, creating parent directories.
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
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
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
