#!/usr/bin/env bash
set -euo pipefail

# Cross-compile the single binaries for macOS, Linux and Windows.
cd "$(dirname "$0")/.."

mkdir -p dist

for os in darwin linux windows; do
  for arch in amd64 arm64; do
    out="dist/agent-session-${os}-${arch}"
    if [ "$os" = "windows" ]; then
      out="${out}.exe"
    fi
    echo "building ${os}/${arch}"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
      go build -trimpath -o "$out" ./cmd/agent-session
  done
done

echo "done: $(ls dist)"
