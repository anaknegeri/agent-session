#!/usr/bin/env bash
set -euo pipefail

# Agent Session installer (PRD §30: curl -fsSL https://install.agent-session.dev | sh)
# Usage: curl -fsSL <install-url> | sh
# Env overrides: AS_VERSION, AS_BASE_URL, AS_INSTALL_DIR

AS_VERSION="${AS_VERSION:-0.1.0}"
AS_BASE_URL="${AS_BASE_URL:-https://github.com/anaknegeri/agent-session/releases/download/v${AS_VERSION}}"
AS_INSTALL_DIR="${AS_INSTALL_DIR:-/usr/local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64)  arch="amd64" ;;
  aarch64) arch="arm64" ;;
  arm64)   arch="arm64" ;;
esac

if [ "$os" = "darwin" ]; then os="darwin"; elif [ "$os" = "linux" ]; then os="linux"; else
  echo "unsupported os: $os" >&2; exit 1
fi

asset="agent-session-${os}-${arch}"
url="${AS_BASE_URL}/${asset}"

echo "installing agent-session ${AS_VERSION} (${os}/${arch}) -> ${AS_INSTALL_DIR}"
if [ -w "$AS_INSTALL_DIR" ]; then
  curl -fsSL "$url" -o "${AS_INSTALL_DIR}/agent-session"
  chmod +x "${AS_INSTALL_DIR}/agent-session"
else
  echo "requesting write access to ${AS_INSTALL_DIR}"
  sudo sh -c "curl -fsSL \"$url\" -o \"${AS_INSTALL_DIR}/agent-session\" && chmod +x \"${AS_INSTALL_DIR}/agent-session\""
fi

echo "installed. try: agent-session doctor"
