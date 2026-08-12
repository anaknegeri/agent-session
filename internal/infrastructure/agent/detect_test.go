package agent

import "testing"

func TestDetectInstalled(t *testing.T) {
	agents := DetectInstalled()
	byName := map[string]bool{}
	for _, a := range agents {
		byName[a.Name] = a.Present
	}

	for _, name := range []string{"claude", "opencode", "codex", "cursor", "cline"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("DetectInstalled missing agent %q", name)
		}
	}

	// Cline is a VS Code extension and never has a CLI binary on PATH.
	if byName["cline"] {
		t.Fatal("cline should never be auto-detected as installed (VS Code extension only)")
	}
}
