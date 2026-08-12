package update

import (
	"context"
	"testing"
)

// TestReleaseServerAllPlatforms verifies the mock release server can resolve a
// checksum for every platform asset, so updateMCP works regardless of the
// runtime GOOS/GOARCH (this is what failed on linux CI runners).
func TestReleaseServerAllPlatforms(t *testing.T) {
	srv, _ := releaseServer(t)
	assetBaseFmt = srv.URL + "/%s/releases/download/%s/%s"
	defer func() { assetBaseFmt = "https://github.com/%s/releases/download/%s/%s" }()

	ctx := context.Background()
	for _, osName := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			ext := ""
			if osName == "windows" {
				ext = ".exe"
			}
			for _, prefix := range []string{"agent-session-", "agent-session-mcp-"} {
				asset := prefix + osName + "-" + arch + ext
				if _, err := fetchChecksum(ctx, "repo", "v1.2.3", asset); err != nil {
					t.Fatalf("fetchChecksum(%s): %v", asset, err)
				}
			}
		}
	}
}
