#!/usr/bin/env bash
set -euo pipefail

# Package the Agent Plugin (agent-plugins.org v1.0.0).
# Usage: scripts/package-plugin.sh [version]
cd "$(dirname "$0")/.."

VERSION="${1:-0.1.0}"
OUT="plugin"

echo "building binaries"
CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/agent-session-mcp" ./cmd/agent-session-mcp

echo "packaging plugin into $OUT"
go run ./cmd/agent-session plugin pack --binary "$OUT/bin/agent-session-mcp" --out "$OUT" --version "$VERSION"

echo "plugin ready:"
ls -lR "$OUT" | head -40
