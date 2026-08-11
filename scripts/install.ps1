<#
.SYNOPSIS
  Installer for Agent Session on Windows.
.DESCRIPTION
  Downloads the prebuilt binaries (agent-session.exe + agent-session-mcp.exe)
  from the latest GitHub release into %LOCALAPPDATA%\agent-session and adds it
  to the user PATH.
.EXAMPLE
  irm https://raw.githubusercontent.com/anaknegeri/agent-session/main/scripts/install.ps1 | iex
.PARAMETER Version
  Specific version to install (default: latest release).
#>
param(
  [string]$Version = "",
  [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"

$Repo = "anaknegeri/agent-session"
$ApiBase = "https://api.github.com/repos/$Repo/releases"

# --- detect latest version -------------------------------------------------
if ([string]::IsNullOrEmpty($Version)) {
  $release = Invoke-RestMethod -Uri "$ApiBase/latest" -Headers @{ "User-Agent" = "agent-session-install" }
  $Version = $release.tag_name.TrimStart("v")
  if ([string]::IsNullOrEmpty($Version)) {
    throw "could not detect latest version; set -Version explicitly"
  }
}
Write-Host "installing agent-session $Version" -ForegroundColor Cyan

# --- detect architecture ---------------------------------------------------
$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -eq "AMD64") { $arch = "amd64" } elseif ($arch -eq "ARM64") { $arch = "arm64" } else {
  throw "unsupported architecture: $arch"
}

# --- install dir -----------------------------------------------------------
if ([string]::IsNullOrEmpty($InstallDir)) {
  $InstallDir = Join-Path $env:LOCALAPPDATA "agent-session"
}
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# --- download binaries -----------------------------------------------------
$base = "https://github.com/$Repo/releases/download/v$Version"
foreach ($name in @("agent-session", "agent-session-mcp")) {
  $asset = "$name-windows-$arch.exe"
  $url = "$base/$asset"
  $dest = Join-Path $InstallDir "$name.exe"
  Write-Host "  downloading $asset"
  Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
}

# --- add to user PATH ------------------------------------------------------
$pathKey = "User"
$userPath = [Environment]::GetEnvironmentVariable("Path", $pathKey)
if ($userPath -notlike "*$InstallDir*") {
  $newPath = if ([string]::IsNullOrEmpty($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
  [Environment]::SetEnvironmentVariable("Path", $newPath, $pathKey)
  Write-Host "  added $InstallDir to user PATH (restart terminal to apply)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "installed to $InstallDir"
Write-Host "open a new terminal and run: agent-session doctor"
