// Package version is the single source of truth for build metadata.
//
// Version, Commit and Date are injected at build time via:
//
//	go build -ldflags "-X github.com/anaknegeri/agent-session/pkg/version.Version=v1.2.3 \
//	                   -X github.com/anaknegeri/agent-session/pkg/version.Commit=abc1234 \
//	                   -X github.com/anaknegeri/agent-session/pkg/version.Date=2026-08-11T00:00:00Z"
//
// The Makefile and scripts/version.sh wire these automatically; the defaults
// below only apply when building without the flags (e.g. `go run`).
package version

import (
	"fmt"
	"runtime"
)

// DefaultVersion is the fallback when not injected. It must match the latest
// tagged release so ad-hoc builds report a sane version.
const DefaultVersion = "0.1.0"

var (
	Version = DefaultVersion
	Commit  = "dev"
	Date    = "unknown"
)

// String returns the semantic version (e.g. "0.1.0").
func String() string {
	return Version
}

// Full returns a single-line human-readable version string.
func Full() string {
	return fmt.Sprintf("agent-session %s (commit %s, built %s, %s/%s)",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
}

// Short returns version plus commit, e.g. "0.1.0 (abc1234)".
func Short() string {
	return fmt.Sprintf("%s (%s)", Version, Commit)
}
