#!/usr/bin/env bash
set -euo pipefail

# Cross-compile agent-session and agent-session-mcp for macOS, Linux, Windows.
# Version metadata is injected via ldflags (see Makefile).
cd "$(dirname "$0")/.."

mkdir -p dist

version_tag="$(./scripts/version.sh)"
version="${version_tag#v}"
commit="$(git rev-parse --short HEAD 2>/dev/null || echo dev)"
date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ldflags="-s -w \
  -X github.com/anaknegeri/agent-session/pkg/version.Version=${version} \
  -X github.com/anaknegeri/agent-session/pkg/version.Commit=${commit} \
  -X github.com/anaknegeri/agent-session/pkg/version.Date=${date}"

for os in darwin linux windows; do
  for arch in amd64 arm64; do
    ext=""
    [ "$os" = "windows" ] && ext=".exe"
    for bin in agent-session agent-session-mcp; do
      out="dist/${bin}-${os}-${arch}${ext}"
      echo "building ${os}/${arch} ${bin} (${version})"
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -trimpath -ldflags "$ldflags" -o "$out" ./cmd/${bin}
    done
  done
done

echo "done: $(ls dist | tr '\n' ' ')"
