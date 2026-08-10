package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAGENTSMD(t *testing.T) {
	dir := t.TempDir()

	path, err := EnsureAGENTSMD(dir)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if path == "" {
		t.Fatalf("expected AGENTS.md to be written")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "session.get") {
		t.Fatalf("expected instructions content")
	}

	// idempotent: second run appends nothing
	path2, err := EnsureAGENTSMD(dir)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if path2 != "" {
		t.Fatalf("expected no change on second run, got path %s", path2)
	}

	// appends to existing file with other content
	existing := filepath.Join(t.TempDir(), "AGENTS.md")
	_ = os.WriteFile(existing, []byte("# My project\n"), 0o644)
	path3, err := EnsureAGENTSMD(filepath.Dir(existing))
	if err != nil {
		t.Fatalf("append ensure: %v", err)
	}
	if path3 == "" {
		t.Fatalf("expected append to write file")
	}
	data3, _ := os.ReadFile(path3)
	if !strings.HasPrefix(string(data3), "# My project\n") {
		t.Fatalf("existing content lost: %s", data3)
	}
	if !strings.Contains(string(data3), "Agent Session") {
		t.Fatalf("section missing: %s", data3)
	}
}
