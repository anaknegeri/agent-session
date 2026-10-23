package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces path's contents in a single step: the bytes go to a
// temp file in the same directory, are flushed, and are then renamed over the
// target.
//
// os.WriteFile truncates before it writes, so a crash — or two `init` runs
// racing each other — can leave ~/.claude.json or a hooks.json holding half a
// document. A half-written config is a parse error, ReadJSONConfig then
// (correctly) refuses to overwrite a file it cannot parse, and the user's own
// MCP servers are unrecoverable without hand-editing. That is the exact loss
// these helpers exist to prevent, so they must not create it themselves.
//
// The temp file shares the target's directory because rename is only atomic
// within a filesystem.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmp := f.Name()
	// Harmless once the rename has succeeded (the name is gone); on every failure
	// path below it is what keeps a stray temp file out of the user's config dir.
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	// Without the flush the rename can be durable while the contents are not,
	// which is the truncated-file failure again by a slower route.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	// CreateTemp makes the file 0600; agents have to be able to read what we wrote.
	if err := os.Chmod(tmp, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
