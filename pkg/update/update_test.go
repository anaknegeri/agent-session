package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// releaseServer serves versioned assets plus their SHA256SUMS.txt for every
// platform (darwin/linux/windows × amd64/arm64) so tests are independent of the
// runtime GOOS/GOARCH. updateMCP resolves the current platform asset, which
// must exist regardless of where the test runs.
func releaseServer(t *testing.T) (*httptest.Server, map[string]string) {
	t.Helper()
	assets := map[string]string{}
	for _, osName := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			ext := ""
			if osName == "windows" {
				ext = ".exe"
			}
			mainAsset := fmt.Sprintf("agent-session-%s-%s%s", osName, arch, ext)
			mcpAsset := fmt.Sprintf("agent-session-mcp-%s-%s%s", osName, arch, ext)
			assets[mainAsset] = sha256Hex("main-" + mainAsset)
			assets[mcpAsset] = sha256Hex("mcp-" + mcpAsset)
		}
	}

	var sums strings.Builder
	for name, sum := range assets {
		fmt.Fprintf(&sums, "%s  %s\n", sum, name)
	}
	assets["SHA256SUMS.txt"] = sums.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if r.URL.Path == "/repo/releases/download/v1.2.3/SHA256SUMS.txt" {
			w.Write([]byte(assets["SHA256SUMS.txt"]))
			return
		}
		switch {
		case strings.HasPrefix(name, "agent-session-mcp-"):
			w.Write([]byte("mcp-" + name))
		case strings.HasPrefix(name, "agent-session-"):
			w.Write([]byte("main-" + name))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, assets
}

func TestUpdateMCP(t *testing.T) {
	srv, _ := releaseServer(t)
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "agent-session")
	mcpPath := filepath.Join(dir, "agent-session-mcp")
	if err := os.WriteFile(mainPath, []byte("old-main"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte("old-mcp"), 0o755); err != nil {
		t.Fatal(err)
	}

	assetBaseFmt = srv.URL + "/%s/releases/download/%s/%s"
	defer func() { assetBaseFmt = "https://github.com/%s/releases/download/%s/%s" }()

	if err := updateMCP(context.Background(), "repo", "1.2.3", dir); err != nil {
		t.Fatalf("updateMCP: %v", err)
	}

	got, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "mcp-agent-session-mcp-") {
		t.Fatalf("mcp binary = %q, want mcp asset content", got)
	}
	info, err := os.Stat(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("mcp binary not executable")
	}
}

// TestUpdateMCPSkipsMissing covers the common case where no agent-session-mcp
// sits next to the main binary: updateMCP must be a no-op, not an error.
func TestUpdateMCPSkipsMissing(t *testing.T) {
	srv, _ := releaseServer(t)
	assetBaseFmt = srv.URL + "/%s/releases/download/%s/%s"
	defer func() { assetBaseFmt = "https://github.com/%s/releases/download/%s/%s" }()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent-session"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := updateMCP(context.Background(), "repo", "1.2.3", dir); err != nil {
		t.Fatalf("updateMCP with missing mcp binary: %v", err)
	}
}

func TestAssetName(t *testing.T) {
	name := assetName()
	if name == "" {
		t.Fatal("assetName returned empty")
	}
}

func TestMCPAssetName(t *testing.T) {
	name := mcpAssetName()
	if name == "" {
		t.Fatal("mcpAssetName returned empty")
	}
	if name == assetName() {
		t.Fatalf("mcp asset must differ from main asset, both %q", name)
	}
	if mcpBinaryName() == "" {
		t.Fatal("mcpBinaryName returned empty")
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
