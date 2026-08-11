# Homebrew formula for Agent Session (macOS + Linux via Linuxbrew).
# Regenerate with: scripts/update-homebrew-formula.sh
# After a tagged release, publish to your tap (e.g. anaknegeri/homebrew-tap).
class AgentSession < Formula
  desc "Universal session & handoff layer for AI coding agents"
  homepage "https://agent-session.dev"
  license "MIT"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-darwin-arm64"
      sha256 "REPLACE_DARWIN_ARM64"
      resource "mcp" do
        url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-mcp-darwin-arm64"
        sha256 "REPLACE_MCP_DARWIN_ARM64"
      end
    else
      url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-darwin-amd64"
      sha256 "REPLACE_DARWIN_AMD64"
      resource "mcp" do
        url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-mcp-darwin-amd64"
        sha256 "REPLACE_MCP_DARWIN_AMD64"
      end
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-linux-arm64"
      sha256 "REPLACE_LINUX_ARM64"
      resource "mcp" do
        url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-mcp-linux-arm64"
        sha256 "REPLACE_MCP_LINUX_ARM64"
      end
    else
      url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-linux-amd64"
      sha256 "REPLACE_LINUX_AMD64"
      resource "mcp" do
        url "https://github.com/anaknegeri/agent-session/releases/download/v0.1.0/agent-session-mcp-linux-amd64"
        sha256 "REPLACE_MCP_LINUX_AMD64"
      end
    end
  end

  def install
    bin.install Dir["agent-session-*"].find { |f| !f.include?("mcp") } => "agent-session"
    resource("mcp").stage { bin.install Dir["agent-session-mcp-*"].first => "agent-session-mcp" }
  end

  test do
    assert_match "agent-session", shell_output("#{bin}/agent-session --help")
  end
end
