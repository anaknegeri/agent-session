#!/usr/bin/env bash
set -euo pipefail

# version.sh — compute the semantic version from git.
#
# Outputs:
#   - the latest semver tag with a leading 'v' (e.g. "v0.1.0") when HEAD is on
#     or past a tag,
#   - otherwise "v0.0.0-dev" so builds are still versioned.
#
# Usage: VERSION="$(scripts/version.sh)"

cd "$(dirname "$0")/.."

if tag="$(git describe --tags --abbrev=0 2>/dev/null)"; then
  echo "$tag"
else
  echo "v0.0.0-dev"
fi
