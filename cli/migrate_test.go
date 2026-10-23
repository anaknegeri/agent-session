package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// fingerprint used to compare file sizes, so `migrate` reported work it had not
// done: an uninstall that replaced a rule or re-indented a config to the same
// length read as "already clean", and a file we really had changed produced no
// "✓ removed" line.
func TestFingerprintSeesContentNotSize(t *testing.T) {
	dir := t.TempDir()
	const probe = "settings.json"
	path := filepath.Join(dir, probe)
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	probes := []string{probe}

	before := fingerprint(dir, probes)
	if err := os.WriteFile(path, []byte(`{"b":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if after := fingerprint(dir, probes); after == before {
		t.Fatalf("a content-only change of the same length was invisible: %q", after)
	}

	// Unchanged content must still read as unchanged, or migrate claims work on
	// every run.
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if again := fingerprint(dir, probes); again != before {
		t.Errorf("identical content produced a different fingerprint: %q vs %q", again, before)
	}

	// A probe that never existed and one that was removed must read the same, so
	// removal registers as a change.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if gone := fingerprint(dir, probes); gone == before {
		t.Error("removing the file did not change the fingerprint")
	}
}
