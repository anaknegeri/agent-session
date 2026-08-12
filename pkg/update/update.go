// Package update implements self-update for agent-session.
//
// It checks the latest GitHub release, compares semver against the running
// binary, downloads the matching platform asset, verifies its SHA256 against
// the release checksum file, and atomically replaces the current executable.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anaknegeri/agent-session/pkg/version"
)

// DefaultRepo is the GitHub repository that hosts the releases.
const DefaultRepo = "anaknegeri/agent-session"

// Base URLs. Kept as variables so tests can override them.
var (
	apiLatestFmt = "https://api.github.com/repos/%s/releases/latest"
	assetBaseFmt = "https://github.com/%s/releases/download/%s/%s"
)

// Info describes the state of an update check.
type Info struct {
	Current     string // installed version
	Latest      string // latest available version
	HasUpdate   bool   // latest > current
	CurrentPath string // path of the running binary
}

// Latest queries the GitHub API for the newest release tag.
func Latest(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf(apiLatestFmt, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "agent-session-update")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("query latest release: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("parse release response: %w", err)
	}
	return strings.TrimPrefix(payload.TagName, "v"), nil
}

// Check reports whether an update is available for the running binary.
func Check(ctx context.Context, repo string) (*Info, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	latest, err := Latest(ctx, repo)
	if err != nil {
		return nil, err
	}
	return &Info{
		Current:     version.Version,
		Latest:      latest,
		HasUpdate:   version.Compare(version.Version, latest) < 0,
		CurrentPath: exe,
	}, nil
}

// Download fetches the platform binary asset for version into path and verifies
// its SHA256 against the release checksum file.
func Download(ctx context.Context, repo, versionTag, path string) error {
	return downloadAsset(ctx, repo, versionTag, assetName(), path)
}

// DownloadMCP fetches the agent-session-mcp platform asset for version into
// path and verifies its SHA256 against the release checksum file.
func DownloadMCP(ctx context.Context, repo, versionTag, path string) error {
	return downloadAsset(ctx, repo, versionTag, mcpAssetName(), path)
}

func downloadAsset(ctx context.Context, repo, versionTag, asset, path string) error {
	url := fmt.Sprintf(assetBaseFmt, repo, versionTag, asset)
	sum, err := fetchChecksum(ctx, repo, versionTag, asset)
	if err != nil {
		return err
	}

	if err := downloadFile(ctx, url, path); err != nil {
		return err
	}
	if err := verifySHA256(path, sum); err != nil {
		os.Remove(path)
		return err
	}
	return os.Chmod(path, 0o755)
}

// SelfUpdate downloads the latest release and replaces the running binary and
// any agent-session-mcp binary sitting next to it. Returns the new version
// installed.
func SelfUpdate(ctx context.Context, repo string, force bool) (string, error) {
	info, err := Check(ctx, repo)
	if err != nil {
		return "", err
	}
	if !info.HasUpdate && !force {
		return "", fmt.Errorf("already up to date (%s)", info.Current)
	}

	dir := filepath.Dir(info.CurrentPath)
	tmp, err := os.CreateTemp(dir, "agent-session-*.new")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	if err := Download(ctx, repo, "v"+info.Latest, tmpPath); err != nil {
		return "", err
	}

	backup := info.CurrentPath + ".old"
	os.Remove(backup)
	if err := os.Rename(info.CurrentPath, backup); err != nil {
		return "", fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(tmpPath, info.CurrentPath); err != nil {
		os.Rename(backup, info.CurrentPath) // rollback
		return "", fmt.Errorf("replace binary: %w", err)
	}
	os.Remove(backup)

	if err := updateMCP(ctx, repo, info.Latest, dir); err != nil {
		return "", err
	}
	return info.Latest, nil
}

// updateMCP replaces the agent-session-mcp binary in dir when it exists,
// keeping it in sync with the main binary. Missing MCP binaries (e.g. a
// user-scope install that only ever runs `mcp` via the main binary) are
// skipped, not an error.
func updateMCP(ctx context.Context, repo, version, dir string) error {
	target := filepath.Join(dir, mcpBinaryName())
	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat agent-session-mcp: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "agent-session-mcp-*.new")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	if err := DownloadMCP(ctx, repo, "v"+version, tmpPath); err != nil {
		return fmt.Errorf("download agent-session-mcp: %w", err)
	}

	backup := target + ".old"
	os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("backup agent-session-mcp: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		os.Rename(backup, target) // rollback
		return fmt.Errorf("replace agent-session-mcp: %w", err)
	}
	os.Remove(backup)
	return nil
}

// assetName returns the release asset name for the current platform.
func assetName() string {
	osName := runtime.GOOS
	if osName == "windows" {
		return fmt.Sprintf("agent-session-windows-%s.exe", runtime.GOARCH)
	}
	return fmt.Sprintf("agent-session-%s-%s", osName, runtime.GOARCH)
}

// mcpAssetName returns the release asset name of the MCP server binary for the
// current platform. Release builds ship agent-session-mcp alongside the main
// binary (see scripts/cross-compile.sh).
func mcpAssetName() string {
	osName := runtime.GOOS
	if osName == "windows" {
		return fmt.Sprintf("agent-session-mcp-windows-%s.exe", runtime.GOARCH)
	}
	return fmt.Sprintf("agent-session-mcp-%s-%s", osName, runtime.GOARCH)
}

// mcpBinaryName is the file name of the MCP server binary next to the main
// executable on the current platform.
func mcpBinaryName() string {
	if runtime.GOOS == "windows" {
		return "agent-session-mcp.exe"
	}
	return "agent-session-mcp"
}

func fetchChecksum(ctx context.Context, repo, tag, asset string) (string, error) {
	url := fmt.Sprintf(assetBaseFmt, repo, tag, "SHA256SUMS.txt")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "agent-session-update")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch checksums: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasSuffix(fields[1], asset) {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %s", asset)
}

func downloadFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "agent-session-update")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, want)
	}
	return nil
}
