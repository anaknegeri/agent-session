package pi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every test here works inside t.TempDir(). Nothing may touch the developer's
// real ~/.pi: an adapter test that rewrites the machine's agent configuration is
// a test that has to be run carefully, which means it stops being run.

func TestEnsureResourcesWritesEverything(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi")
	bin := "/usr/local/bin/agent-session"

	if err := EnsureResources(root, bin); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}

	ext := read(t, extensionPath(root))
	if !strings.Contains(ext, `"`+bin+`"`) {
		t.Errorf("the extension does not carry the binary path %q:\n%s", bin, ext)
	}
	if strings.Contains(ext, binPlaceholder) {
		t.Error("the binary placeholder was left unreplaced, so the extension cannot run")
	}
	if !strings.Contains(ext, managedMarker) {
		t.Error("the extension has no managed marker, so uninstall will not remove it")
	}

	skill := read(t, filepath.Join(skillDir(root), "SKILL.md"))
	// pi drops a skill with no description instead of reporting it, so an empty
	// frontmatter would install silently and never load.
	if !strings.Contains(skill, "\nname: agent-session\n") {
		t.Error("SKILL.md has no name in its frontmatter")
	}
	if !strings.Contains(skill, "\ndescription: ") {
		t.Error("SKILL.md has no description; pi would not load it")
	}

	for _, c := range Commands() {
		path := filepath.Join(PromptsDir(root), c.FileName())
		body := read(t, path)
		if !strings.HasPrefix(body, "---\n") {
			t.Errorf("%s does not start with frontmatter, pi will not parse it", path)
		}
	}
}

// TestPromptsMentionNoMCPTools is the point of pi having its own command set. A
// prompt telling the agent to call session.get sends it after a tool pi has no
// way to reach.
func TestPromptsMentionNoMCPTools(t *testing.T) {
	forbidden := []string{"session.get", "context.get", "session.record", "session.checkpoint", "MCP"}
	for _, c := range Commands() {
		for _, bad := range forbidden {
			if strings.Contains(c.Prompt, bad) {
				t.Errorf("prompt %q references %q, but pi has no MCP client", c.Name, bad)
			}
		}
	}
	if strings.Contains(skillMD, "MCP tool") {
		t.Error("SKILL.md points the agent at MCP tools pi cannot call")
	}
}

func TestEnsureResourcesIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi")
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	before := read(t, extensionPath(root))
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if after := read(t, extensionPath(root)); after != before {
		t.Error("re-running setup changed the extension")
	}
}

// TestEnsureResourcesLeavesForeignFiles covers the case that actually costs a
// user something: a hand-written extension or prompt of the same name.
func TestEnsureResourcesLeavesForeignFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi")
	const mine = "// my own extension\n"
	if err := os.MkdirAll(filepath.Dir(extensionPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extensionPath(root), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}
	if got := read(t, extensionPath(root)); got != mine {
		t.Errorf("setup overwrote an extension it does not own:\n%s", got)
	}

	if err := RemoveResources(root); err != nil {
		t.Fatalf("remove resources: %v", err)
	}
	if got := read(t, extensionPath(root)); got != mine {
		t.Error("uninstall removed an extension it does not own")
	}
}

func TestRemoveResources(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi")
	if err := EnsureResources(root, "agent-session"); err != nil {
		t.Fatalf("ensure resources: %v", err)
	}
	if err := RemoveResources(root); err != nil {
		t.Fatalf("remove resources: %v", err)
	}

	gone := []string{
		extensionPath(root),
		filepath.Join(skillDir(root), "SKILL.md"),
	}
	for _, c := range Commands() {
		gone = append(gone, filepath.Join(PromptsDir(root), c.FileName()))
	}
	for _, path := range gone {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", path)
		}
	}

	// Removing twice must not fail: `plugin uninstall` is re-runnable.
	if err := RemoveResources(root); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}

func TestAdapterUsesProjectScopePaths(t *testing.T) {
	project := t.TempDir()
	a := NewAdapter(project)

	if a.Name() != "pi" {
		t.Errorf("adapter name is %q", a.Name())
	}
	if found, _ := a.Detect(context.Background()); found {
		t.Error("detected pi in a project with no .pi directory")
	}

	if err := a.Configure(context.Background(), "agent-session"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := a.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Project scope is .pi/, user scope is ~/.pi/agent/ — the asymmetry is pi's,
	// and getting it wrong installs files pi never looks at.
	if _, err := os.Stat(filepath.Join(project, ".pi", "extensions", "agent-session.ts")); err != nil {
		t.Errorf("extension not at .pi/extensions: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".pi", "agent")); err == nil {
		t.Error("project scope must not nest under .pi/agent; that is the user-scope layout")
	}
	if found, _ := a.Detect(context.Background()); !found {
		t.Error("pi not detected after install")
	}

	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".pi", "extensions", "agent-session.ts")); !os.IsNotExist(err) {
		t.Error("extension survived uninstall")
	}
}

func TestUserRootIsPiAgentDir(t *testing.T) {
	if got, want := UserRoot("/home/u"), filepath.Join("/home/u", ".pi", "agent"); got != want {
		t.Errorf("UserRoot = %q, want %q", got, want)
	}
	if got, want := ProjectRoot("/w/proj"), filepath.Join("/w/proj", ".pi"); got != want {
		t.Errorf("ProjectRoot = %q, want %q", got, want)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
