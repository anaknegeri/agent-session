package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
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

// The ownership predicate used to live here and match the full literal
// `<!-- agent-session:managed -->`. The moment the marker grew a version and a
// digest, every command file the new build wrote failed its own check: init
// stopped updating them and uninstall left them behind. Ownership now has exactly
// one definition, in the agent package.
func TestInstallUpdatesFilesWrittenByAnOlderRelease(t *testing.T) {
	dir := t.TempDir()
	first := All()[0]
	path := filepath.Join(dir, first.FileName())
	// What 0.1.8 wrote: our bare marker, an out-of-date body, no digest.
	stale := "---\ndescription: " + first.Description + "\n---\n<!-- " + agent.ManagedMarker + " -->\n\nold prompt\n"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "old prompt") {
		t.Fatalf("a command file of ours was never updated:\n%s", data)
	}

	// Install also wrote the rest of the set, so uninstall must reclaim all of it.
	removed, err := Uninstall(dir)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if removed != len(All()) {
		t.Fatalf("uninstall removed %d of its own files, want %d", removed, len(All()))
	}
}

func TestInstallKeepsAndReportsAnEditedCommand(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	path := filepath.Join(dir, All()[0].FileName())
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "\nAlso always run my linter.\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Install(dir)
	if !errors.Is(err, agent.ErrUserModified) {
		t.Fatalf("an edited command was not reported: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != edited {
		t.Errorf("the user's edit was overwritten:\n%s", got)
	}
	// The rest of the set still installs: one tuned command must not block them.
	for _, c := range All()[1:] {
		if _, statErr := os.Stat(filepath.Join(dir, c.FileName())); statErr != nil {
			t.Errorf("%s missing after a conflict on another file: %v", c.FileName(), statErr)
		}
	}

	removed, err := Uninstall(dir)
	if !errors.Is(err, agent.ErrUserModified) {
		t.Fatalf("uninstall did not report the file it kept: %v", err)
	}
	if removed != len(All())-1 {
		t.Errorf("uninstall removed %d files, want %d", removed, len(All())-1)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("uninstall deleted the command the user had edited: %v", statErr)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
