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
	"strings"
)

// DefaultVersion is the fallback when not injected. It must match the latest
// tagged release so ad-hoc builds report a sane version.
const DefaultVersion = "0.1.7"

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

// Compare compares two semantic versions. Returns -1 if a < b, 0 if equal,
// 1 if a > b. Pre-release suffixes (e.g. "0.1.0-dev") sort below the release.
func Compare(a, b string) int {
	return compare(parse(a), parse(b))
}

type semver struct {
	major, minor, patch int
	pre                 string
}

func parse(v string) semver {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, "-", 2)
	s := semver{}
	nums := strings.Split(parts[0], ".")
	if len(nums) > 0 {
		fmt.Sscanf(nums[0], "%d", &s.major)
	}
	if len(nums) > 1 {
		fmt.Sscanf(nums[1], "%d", &s.minor)
	}
	if len(nums) > 2 {
		fmt.Sscanf(nums[2], "%d", &s.patch)
	}
	if len(parts) > 1 {
		s.pre = parts[1]
	}
	return s
}

func compare(a, b semver) int {
	if a.major != b.major {
		return cmp(a.major, b.major)
	}
	if a.minor != b.minor {
		return cmp(a.minor, b.minor)
	}
	if a.patch != b.patch {
		return cmp(a.patch, b.patch)
	}
	switch {
	case a.pre == "" && b.pre == "":
		return 0
	case a.pre == "":
		return 1 // release > pre-release
	case b.pre == "":
		return -1
	case a.pre < b.pre:
		return -1
	case a.pre > b.pre:
		return 1
	default:
		return 0
	}
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
