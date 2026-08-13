package agent

import (
	"os/exec"
)

// InstalledAgent describes a detected coding agent CLI.
type InstalledAgent struct {
	Name    string
	Present bool
}

// DetectInstalled checks which agent CLIs are available on PATH. `init` wires
// only these, so it never creates config folders for agents the user does not
// use.
func DetectInstalled() []InstalledAgent {
	return []InstalledAgent{
		{Name: "claude", Present: lookPath("claude")},
		{Name: "opencode", Present: lookPath("opencode")},
		{Name: "codex", Present: lookPath("codex")},
		{Name: "cursor", Present: lookPath("cursor-agent")},
		{Name: "pi", Present: lookPath("pi")},
		{Name: "cline", Present: false}, // Cline is a VS Code extension, no CLI
	}
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
