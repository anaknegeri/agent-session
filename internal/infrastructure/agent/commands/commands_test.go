package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndUninstall(t *testing.T) {
	dir := t.TempDir()

	written, err := Install(dir)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(written) != len(All()) {
		t.Fatalf("expected %d written, got %d", len(All()), len(written))
	}

	// files exist and are non-empty
	for _, c := range All() {
		data, err := os.ReadFile(filepath.Join(dir, c.FileName()))
		if err != nil {
			t.Fatalf("read %s: %v", c.FileName(), err)
		}
		if !strings.Contains(string(data), "Agent Session") {
			t.Fatalf("%s missing Agent Session marker", c.FileName())
		}
	}

	// idempotent: second install writes nothing new
	written2, err := Install(dir)
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if len(written2) != 0 {
		t.Fatalf("expected no new files on reinstall, got %d", len(written2))
	}

	// user modification is preserved (not managed by us)
	userFile := filepath.Join(dir, "agent-session.md")
	if err := os.WriteFile(userFile, []byte("# My custom /agent-session\nAgent Session custom content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir); err != nil {
		t.Fatalf("install over user file: %v", err)
	}
	data, _ := os.ReadFile(userFile)
	if !strings.Contains(string(data), "My custom") {
		t.Fatalf("user file was overwritten: %s", data)
	}

	// uninstall removes ours but keeps user file
	removed, err := Uninstall(dir)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if removed != len(All())-1 {
		t.Fatalf("expected %d removed, got %d", len(All())-1, removed)
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("user file should survive uninstall: %v", err)
	}
}

func TestRender(t *testing.T) {
	for _, c := range All() {
		out := c.Render()
		// frontmatter must be the first thing (opencode/cursor parse it)
		if !strings.HasPrefix(out, "---\ndescription:") {
			t.Fatalf("%s must start with frontmatter, got: %q", c.FileName(), out[:min(len(out), 40)])
		}
		if !strings.Contains(out, "<!-- agent-session:managed -->") {
			t.Fatalf("%s missing managed marker", c.FileName())
		}
		if !strings.Contains(out, "Agent Session") {
			t.Fatalf("%s missing Agent Session mention", c.FileName())
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
