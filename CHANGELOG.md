# Changelog

All notable changes to Agent Session are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `agent-session migrate` — removes old per-project agent configs and re-wires at user scope
- CI workflow (`ci.yml`) — runs `go test`, `go vet`, `go build` on every push/PR
- Auto-update Homebrew formula on release (CI builds bottle + pushes to tap)
- CONTRIBUTING.md
- **Auto-record file changes** — `context.get` compares git status against recorded events and appends `file.changed` for unrecorded files
- **Auto-checkpoint when stale** — no checkpoint in 10 min + dirty tree triggers one on `context.get`
- **Context nudges** — summary warns about stale checkpoints, unrecorded files, and open blockers
- **`session.record`** — unified tool: event + decision + next_action + checkpoint in one call

### Fixed
- `git diff HEAD` did not detect untracked files, so `workspace.status`/`workspace.diff` reported `dirty:false` for new files

### Changed
- GitHub Actions upgraded to Node.js 24 (checkout@v5, setup-go@v6, upload/download-artifact@v5)
- Release workflow Go version aligned with go.mod (1.25)
- MCP server instructions updated for `session.record` and auto-recording

## [0.1.4] — 2026-08-12

### Added
- Complete user-scope wiring: Claude Code hooks (`SessionStart`/`Stop`/`PreCompact`) merged into `~/.claude/settings.json`, Agent Session rule appended to `~/.claude/CLAUDE.md`, OpenCode global instructions in `~/.config/opencode/opencode.json`
- `plugin uninstall --scope user` — reverses what `init` wired at user scope
- Validation: `task.create` rejects empty titles, `event.append` rejects unknown event types, `blocker.resolve` returns `ErrBlockerNotFound`, `handoff` rejects unsupported agents
- Bounded handoff output (applies context budget — max decisions/blockers/files, per-item truncation)
- Tests: `bugfix_test.go`, `claude/global_test.go`, `opencode/global_test.go`

### Changed
- `init` default wires agents at user scope (no per-project config pollution)
- `init` detects installed agent CLIs and wires only those
- `status` collapses newlines in free-text session titles

## [0.1.3] — 2026-08-11

### Added
- MCP server declares `instructions` field in `InitializeResult` so agents (Claude Code, OpenCode) proactively discover the session workflow without prompting

## [0.1.2] — 2026-08-11

### Added
- Centralized versioning (`pkg/version`) with ldflags injection (version, commit, build date)
- Self-update: `agent-session update` checks GitHub releases, downloads, verifies SHA256, replaces binary atomically
- `agent-session update --check` — report-only mode
- MIT LICENSE file
- Homebrew bottle support for macOS arm64

### Changed
- Release workflow injects correct version/commit/date via ldflags
- Removed Roadmap section from README

## [0.1.1] — 2026-08-11

### Fixed
- Release workflow builds both `agent-session` and `agent-session-mcp` for all 6 platforms
- Homebrew formula generator fetches checksums from GitHub release (source of truth)
- Homebrew formula filename convention fixed (single dash)

## [0.1.0] — 2026-08-11

### Added
- `agent-session diff` — compare two checkpoints (new decisions, resolved blockers, task status, files, commits) + MCP `session.diff` tool
- `agent-session projects` — global registry listing all initialized projects with session status
- `agent-session export/import` — full session state to JSON or Markdown
- Cursor adapter (`.cursor/mcp.json` + rules) and Cline adapter (`.clinerules` + `.vscode/settings.json`)
- `agent-session ui` — full dashboard with progress bar, checkpoints, events
- `agent-session stats` — task/decision/blocker/test/agent insights
- `agent-session timeline` — visual event timeline with icons
- Smart auto-checkpoint — rate-limits to 1 per 60 seconds
- `agent-session watch` — polling mode that regenerates context.md on change
- Token benchmark script (`bench/token-benchmark.sh`) with live measurements
- Multi-platform install: Homebrew tap, curl installer (macOS/Linux), PowerShell installer (Windows), `go install`

### Fixed
- Test status now reflects the most recent test event, not a stale one
- Staged changes visible in workspace status/diff (`git diff HEAD`)
- Project paths canonicalized with `filepath.EvalSymlinks` (macOS `/var` → `/private/var`)
