package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGlobalHooks(t *testing.T) {
	home := t.TempDir()

	if err := EnsureGlobalHooks(home); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	for _, event := range []string{"SessionStart", "Stop", "PreCompact"} {
		if !hasAgentSessionHook(hooks, event) {
			t.Fatalf("expected %s hook to be present", event)
		}
	}

	// idempotent: re-running does not duplicate entries
	if err := EnsureGlobalHooks(home); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	data2, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	var settings2 map[string]any
	if err := json.Unmarshal(data2, &settings2); err != nil {
		t.Fatalf("parse settings.json (2nd): %v", err)
	}
	hooks2, _ := settings2["hooks"].(map[string]any)
	entries, _ := hooks2["SessionStart"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 SessionStart entry after re-run, got %d", len(entries))
	}

	// unrelated existing settings survive a merge
	other := filepath.Join(t.TempDir(), ".claude")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"permissions":{"allow":["Bash(git status)"]}}`
	if err := os.WriteFile(filepath.Join(other, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGlobalHooks(filepath.Dir(other)); err != nil {
		t.Fatalf("merge ensure: %v", err)
	}
	merged, _ := os.ReadFile(filepath.Join(other, "settings.json"))
	if !strings.Contains(string(merged), "Bash(git status)") {
		t.Fatalf("existing permissions lost: %s", merged)
	}
	if !strings.Contains(string(merged), "agent-session") {
		t.Fatalf("hooks missing after merge: %s", merged)
	}
}

func TestRemoveGlobalHooks(t *testing.T) {
	// no-op when the file doesn't exist
	if err := RemoveGlobalHooks(t.TempDir()); err != nil {
		t.Fatalf("remove on missing file: %v", err)
	}

	home := t.TempDir()
	if err := EnsureGlobalHooks(home); err != nil {
		t.Fatal(err)
	}
	// a hook belonging to another tool must survive removal
	path := filepath.Join(home, ".claude", "settings.json")
	data, _ := os.ReadFile(path)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	hooks := settings["hooks"].(map[string]any)
	hooks["Notification"] = []any{
		map[string]any{
			"matcher": "*",
			"hooks":   []any{map[string]any{"type": "command", "command": "say done"}},
		},
	}
	out, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveGlobalHooks(home); err != nil {
		t.Fatalf("remove: %v", err)
	}
	data2, _ := os.ReadFile(path)
	var settings2 map[string]any
	if err := json.Unmarshal(data2, &settings2); err != nil {
		t.Fatal(err)
	}
	hooks2, _ := settings2["hooks"].(map[string]any)
	if hasAgentSessionHook(hooks2, "SessionStart") || hasAgentSessionHook(hooks2, "Stop") || hasAgentSessionHook(hooks2, "PreCompact") {
		t.Fatalf("expected agent-session hooks to be removed: %s", data2)
	}
	if _, ok := hooks2["Notification"]; !ok {
		t.Fatalf("expected unrelated Notification hook to survive: %s", data2)
	}
}

func TestEnsureGlobalRule(t *testing.T) {
	home := t.TempDir()

	path, err := EnsureGlobalRule(home)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if path == "" {
		t.Fatalf("expected CLAUDE.md to be written")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), ".agent/") {
		t.Fatalf("expected .agent/ guard in rule text: %s", data)
	}

	// idempotent
	path2, err := EnsureGlobalRule(home)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if path2 != "" {
		t.Fatalf("expected no change on second run, got %s", path2)
	}

	// appends to existing CLAUDE.md without losing content
	other := t.TempDir()
	claudeMD := filepath.Join(other, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(claudeMD), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeMD, []byte("# My global preferences\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path3, err := EnsureGlobalRule(other)
	if err != nil {
		t.Fatalf("append ensure: %v", err)
	}
	if path3 == "" {
		t.Fatalf("expected append to write file")
	}
	data3, _ := os.ReadFile(path3)
	if !strings.HasPrefix(string(data3), "# My global preferences\n") {
		t.Fatalf("existing content lost: %s", data3)
	}
}

func TestRemoveGlobalRule(t *testing.T) {
	// no-op when the file doesn't exist
	if err := RemoveGlobalRule(t.TempDir()); err != nil {
		t.Fatalf("remove on missing file: %v", err)
	}

	home := t.TempDir()
	if _, err := EnsureGlobalRule(home); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGlobalRule(home); err != nil {
		t.Fatalf("remove: %v", err)
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), ruleSection) {
		t.Fatalf("expected section to be removed: %s", data)
	}

	// removal preserves other content when the rule was appended
	other := t.TempDir()
	claudeMD := filepath.Join(other, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(claudeMD), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeMD, []byte("# My global preferences\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureGlobalRule(other); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGlobalRule(other); err != nil {
		t.Fatalf("remove after append: %v", err)
	}
	final, _ := os.ReadFile(claudeMD)
	if string(final) != "# My global preferences\n" {
		t.Fatalf("expected only original content to remain, got: %q", final)
	}
}
