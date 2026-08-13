package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of ReadJSONConfig is the difference between "no file yet" and
// "a file I could not parse". Every adapter writes into config the user also
// edits by hand, so collapsing those two cases into an empty map is what turns a
// typo into deleted MCP servers.
func TestReadJSONConfigDistinguishesAbsentFromBroken(t *testing.T) {
	dir := t.TempDir()

	absent, err := ReadJSONConfig(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatalf("absent file: %v", err)
	}
	if len(absent) != 0 {
		t.Errorf("absent file returned %v, want an empty config", absent)
	}

	blank := filepath.Join(dir, "blank.json")
	if err := os.WriteFile(blank, []byte("\n  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if config, err := ReadJSONConfig(blank); err != nil || len(config) != 0 {
		t.Errorf("blank file: config=%v err=%v, want empty config and no error", config, err)
	}

	// JSONC: legal in .vscode/settings.json, rejected by encoding/json. This is the
	// case that has to fail loudly rather than hand back an empty config.
	jsonc := filepath.Join(dir, "settings.json")
	const withComment = "{\n  // mine\n  \"editor.formatOnSave\": true,\n}\n"
	if err := os.WriteFile(jsonc, []byte(withComment), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJSONConfig(jsonc); err == nil {
		t.Error("a JSONC file parsed as strict JSON; callers would overwrite it")
	} else if !strings.Contains(err.Error(), jsonc) {
		t.Errorf("the error does not name the file: %v", err)
	}
	if got := readFile(t, jsonc); got != withComment {
		t.Errorf("ReadJSONConfig modified the file:\n%s", got)
	}

	null := filepath.Join(dir, "null.json")
	if err := os.WriteFile(null, []byte("null"), 0o644); err != nil {
		t.Fatal(err)
	}
	// `null` parses into a nil map; a caller writing into it would panic.
	config, err := ReadJSONConfig(null)
	if err != nil {
		t.Fatalf("null document: %v", err)
	}
	config["x"] = 1
}

func TestWriteJSONConfigDoesNotEscapeCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	const command = `[ -d "$CLAUDE_PROJECT_DIR/.agent" ] && agent-session resume || true`
	if err := WriteJSONConfig(path, map[string]any{"command": command}); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw := readFile(t, path)
	// The quotes inside the command are escaped by any JSON encoder, and must be.
	// What must NOT happen is Go's default HTML escaping turning && into
	// \u0026\u0026: valid JSON that reads like corruption in a file the user opens.
	for _, escaped := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if strings.Contains(raw, escaped) {
			t.Errorf("the hook command carries HTML escapes (%s):\n%s", escaped, raw)
		}
	}
	if !strings.Contains(raw, "&& agent-session resume || true") {
		t.Errorf("the shell operators did not survive the round trip:\n%s", raw)
	}
	if !strings.HasSuffix(raw, "\n") {
		t.Error("no trailing newline; the file is awkward to diff and to append to")
	}

	config, err := ReadJSONConfig(path)
	if err != nil {
		t.Fatalf("round-trip read: %v", err)
	}
	if config["command"] != command {
		t.Errorf("round-trip changed the value: %v", config["command"])
	}
}

func TestSectionCreatesAndPreserves(t *testing.T) {
	config := map[string]any{"mcpServers": map[string]any{"mine": map[string]any{}}}

	servers := Section(config, "mcpServers")
	if _, ok := servers["mine"]; !ok {
		t.Error("Section replaced an existing map instead of returning it")
	}
	Section(servers, "ours")["command"] = "agent-session"
	if got := config["mcpServers"].(map[string]any)["ours"].(map[string]any)["command"]; got != "agent-session" {
		t.Errorf("edits through Section are not visible in the parent config: %v", got)
	}

	// A key holding a non-object is the corrupt case; callers need a usable map
	// rather than a panic.
	broken := map[string]any{"mcpServers": "not an object"}
	Section(broken, "mcpServers")["ours"] = 1
	if _, ok := broken["mcpServers"].(map[string]any); !ok {
		t.Error("Section did not replace a non-object value with a map")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
