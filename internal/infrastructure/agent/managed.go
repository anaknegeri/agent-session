package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ManagedMarker appears in every file agent-session writes into an agent's
// configuration directory. It is the whole ownership model: a file carrying it
// is ours to rewrite or delete, and a file without it belongs to the user and is
// left exactly as found. Adapters that install resources into shared
// directories (pi, omp) share these helpers so "managed" means one thing.
const ManagedMarker = "agent-session:managed"

// WriteManaged writes content at path, creating parent directories. An existing
// file without the marker is left untouched — overwriting a hand-written
// extension or skill costs the user work that setup has no way to give back.
func WriteManaged(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !strings.Contains(string(existing), ManagedMarker) {
			return nil
		}
		if string(existing) == content {
			return nil
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// RemoveManaged deletes path only if it is ours. A missing file is not an error:
// uninstall has to be re-runnable.
func RemoveManaged(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if !strings.Contains(string(data), ManagedMarker) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
