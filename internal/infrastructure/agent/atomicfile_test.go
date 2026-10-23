package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// A config that holds secrets is 0600 for a reason. Rewriting it through the
// temp-file-and-rename path must not hand it the default mode instead.
func TestWriteFileAtomicKeepsExistingMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("api_key = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("api_key = \"y\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after rewrite = %o, want 600 (the file's own mode)", got)
	}
}

func TestWriteFileAtomicNewFileUsesPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := WriteFileAtomic(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("mode of a new file = %o, want 644", got)
	}
}
