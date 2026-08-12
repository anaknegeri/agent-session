package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/infrastructure/agent"
)

// Adapter configures OpenAI Codex through `codex mcp add`, which writes the
// server into the Codex config for us and honours CODEX_HOME.
type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string { return "codex" }

func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	_, err := exec.LookPath("codex")
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a *Adapter) Configure(ctx context.Context, mcpCommand string) error {
	cmd := exec.CommandContext(ctx, "codex", "mcp", "add", "agent-session", "--", mcpCommand, "mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codex mcp add: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) Install(ctx context.Context) error {
	return nil
}

func (a *Adapter) Uninstall(ctx context.Context) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	cfg := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		return nil
	}
	out := removeMCPSection(string(data), "agent-session")
	return os.WriteFile(cfg, []byte(out), 0o644)
}

// configDir resolves Codex's config directory the same way Codex does, so an
// uninstall edits the config that `codex mcp add` actually wrote.
func configDir() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func removeMCPSection(toml, server string) string {
	lines := strings.Split(toml, "\n")
	var result []string
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[mcp_servers."+server+"]") {
			skip = true
			continue
		}
		if skip {
			if trimmed == "" || strings.HasPrefix(trimmed, "[") {
				skip = false
			} else {
				continue
			}
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

var _ agent.Adapter = (*Adapter)(nil)
