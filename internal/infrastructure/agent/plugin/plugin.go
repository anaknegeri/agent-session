package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	pluginSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	mcpSchema    = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
	// ExtensionNamespace is a reverse-domain namespace owned by Agent Session.
	ExtensionNamespace = "dev.agentsession"
)

// Pack materializes an Agent Plugin directory (agent-plugins.org v1.0.0):
// plugin.json + mcp.json (stdio -> ./bin/agent-session-mcp) at dir.
func Pack(dir, binaryPath, version string) error {
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	binDest := filepath.Join(dir, "bin", "agent-session-mcp")
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	if err := os.WriteFile(binDest, data, 0o755); err != nil {
		return fmt.Errorf("write plugin binary: %w", err)
	}

	manifest := map[string]any{
		"$schema":     pluginSchema,
		"name":        "agent-session",
		"version":     version,
		"description": "Universal session & handoff layer for AI coding agents",
		"license":     "MIT",
		"extensions": map[string]any{
			ExtensionNamespace: map[string]any{
				"agent_adapters": []string{"claude", "codex", "opencode"},
			},
		},
	}
	if err := writeJSON(filepath.Join(dir, "plugin.json"), manifest); err != nil {
		return err
	}

	mcpConfig := map[string]any{
		"$schema": mcpSchema,
		"mcpServers": map[string]any{
			"agent-session": map[string]any{
				"type":    "stdio",
				"command": "./bin/agent-session-mcp",
				"cwd":     "${PLUGIN_ROOT}",
			},
		},
	}
	if err := writeJSON(filepath.Join(dir, "mcp.json"), mcpConfig); err != nil {
		return err
	}

	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
