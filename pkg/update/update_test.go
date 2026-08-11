package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	name := assetName()
	if name == "" {
		t.Fatal("assetName returned empty")
	}
}

// TestLatest parses the GitHub API response shape.
func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v0.2.0"}`))
	}))
	defer srv.Close()

	// point at the test server instead of the real API
	apiLatestFmt = srv.URL + "/%s"
	defer func() { apiLatestFmt = "https://api.github.com/repos/%s/releases/latest" }()

	got, err := Latest(context.Background(), "test/repo")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got != "0.2.0" {
		t.Fatalf("Latest = %q, want 0.2.0", got)
	}
}

// TestVerifySHA256 confirms checksum verification works.
func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	if err := verifySHA256(path, sum); err != nil {
		t.Fatalf("verifySHA256 valid: %v", err)
	}
	if err := verifySHA256(path, "0000"); err == nil {
		t.Fatal("verifySHA256 accepted a wrong checksum")
	}
}
