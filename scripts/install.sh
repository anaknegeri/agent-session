#!/usr/bin/env bash
set -euo pipefail

# Agent Session installer for macOS & Linux.
# Usage:   curl -fsSL https://raw.githubusercontent.com/anaknegeri/agent-session/main/scripts/install.sh | sh
# Or:      curl -fsSL https://install.agent-session.dev | sh
# Env:     AS_VERSION    (default: latest GitHub release)
#          AS_BASE_URL   (default: GitHub releases download base)
#          AS_INSTALL_DIR (default: /usr/local/bin, falls back to ~/.local/bin)

AS_BASE_URL="${AS_BASE_URL:-https://github.com/anaknegeri/agent-session/releases/download}"

# Detect latest version from the GitHub API when not pinned.
if [ -z "${AS_VERSION:-}" ]; then
  AS_VERSION="$(curl -fsSL https://api.github.com/repos/anaknegeri/agent-session/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')"
  if [ -z "$AS_VERSION" ]; then
    echo "error: could not detect latest version; set AS_VERSION explicitly" >&2
    exit 1
  fi
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64)  arch="amd64" ;;
  aarch64) arch="arm64" ;;
  arm64)   arch="arm64" ;;
esac

if [ "$os" = "darwin" ]; then os="darwin"; elif [ "$os" = "linux" ]; then os="linux"; else
  echo "unsupported os: $os (use install.ps1 on Windows)" >&2; exit 1
fi

# Choose a writable install dir.
if [ -z "${AS_INSTALL_DIR:-}" ]; then
  if [ -w "/usr/local/bin" ]; then
    AS_INSTALL_DIR="/usr/local/bin"
  else
    AS_INSTALL_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$AS_INSTALL_DIR"

version_tag="v${AS_VERSION}"

install_bin() {
  local name="$1"
  local asset="${name}-${os}-${arch}"
  local url="${AS_BASE_URL}/${version_tag}/${asset}"
  echo "  downloading ${asset} (${AS_VERSION})"
  if [ -w "$AS_INSTALL_DIR" ]; then
    curl -fsSL "$url" -o "${AS_INSTALL_DIR}/${name}"
    chmod +x "${AS_INSTALL_DIR}/${name}"
  else
    sudo sh -c "curl -fsSL \"$url\" -o \"${AS_INSTALL_DIR}/${name}\" && chmod +x \"${AS_INSTALL_DIR}/${name}\""
  fi
}

echo "installing agent-session ${AS_VERSION} (${os}/${arch}) -> ${AS_INSTALL_DIR}"
install_bin "agent-session"
install_bin "agent-session-mcp"
echo ""
echo "installed. try: agent-session doctor"
