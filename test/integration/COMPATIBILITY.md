# Agent Compatibility Matrix

Capabilities per agent. Cells state how the capability is backed, not how well
it is expected to work: ✅ covered by an automated test · 🟡 wired and manually
exercised, no automated coverage · ⚠ known gap · ❌ not supported.

Only the transport is agent-independent, so most cells are 🟡 by design — the
gated smoke tests need the real CLIs plus credentials and cannot run in ordinary
CI. Do not upgrade a cell to ✅ without a test that fails when the capability
breaks.

| Agent | MCP tools | Context | Checkpoint | Handoff | Hooks | Slash cmds |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| Claude Code | ✅ | ✅ | ✅ | 🟡 | ✅ | 🟡 |
| Codex | ✅ | 🟡 | 🟡 | 🟡 | 🟡 wired, end-to-end unverified | — |
| OpenCode | 🟡 | 🟡 | 🟡 | 🟡 | ⚠ via instructions only | 🟡 |
| Cursor | 🟡 | 🟡 | 🟡 | 🟡 | ⚠ via rules only | 🟡 |
| Cline | 🟡 | 🟡 | 🟡 | ⚠ | ⚠ via rules only | — |

## Legend

- **MCP tools** — the agent can call agent-session MCP tools over stdio.
- **Context** — `context.get` output is usable and nudges surface.
- **Checkpoint** — `session.checkpoint` / hooks produce snapshots.
- **Handoff** — `handoff` context is produced and consumed by the next agent.
- **Hooks** — SessionStart/Stop/PreCompact fire automatically.
- **Slash cmds** — `/agent-session*` commands are available.

## Verified by test

| Test | Agent | What it proves | Gated |
|---|---|---|---|
| `TestRealMCPOverStdio` | any stdio MCP client | session.get → context.get → task.create → session.record → checkpoint, over a real stdio subprocess | no |
| `TestClaudeCodeSmoke` | Claude Code | `init --project --only claude` writes `.mcp.json`, `.claude/settings.json` (with SessionStart/Stop/PreCompact) and `.claude/CLAUDE.md`; a real run appends events and produces a Stop-hook checkpoint (label `auto`) | yes |
| `TestOpenCodeSmoke` | OpenCode | the generated `opencode.json` is valid enough for the CLI to start and the session survives the run. OpenCode has no hooks, so state movement is model-dependent and is not asserted | yes |
| `TestCodexSmoke` | Codex | `init --only codex` registers `[mcp_servers.agent-session]` in the config under `CODEX_HOME` (isolated in a temp dir, so this is attributable); a real `codex exec` run then leaves the session store intact | yes |

### Attribution caveat for the Claude smoke test

Claude merges user-scope and project-scope settings. On a machine where
`agent-session init` has already wired `~/.claude/settings.json`, those hooks fire
in any project containing `.agent/`, so a passing behavioural assertion does not
prove the project-scope wiring works. The file-level assertions are
scope-attributable; the behavioural ones are not, and the test logs a note when it
detects user-scope wiring. Run on a clean machine or in CI to exercise the
project-scope path.

Verified on 12 August 2026, macOS, against the real CLIs: `TestClaudeCodeSmoke`
(Claude Code), `TestOpenCodeSmoke` (OpenCode 1.18.15) and `TestCodexSmoke`
(codex-cli 0.147.0) all pass. The Claude run had user-scope wiring present, so
its behavioural half carries the caveat above. The Codex run needed one retry:
the first attempt hit an upstream `Selected model is at capacity` error and
skipped, which is the intended behaviour of the skip path.

### Not covered

- **Codex hooks firing.** `init --only codex` now writes `SessionStart` and `Stop`
  hooks into `$CODEX_HOME/hooks.json`, using the same schema Codex plugins use
  (verified against an installed plugin's `hooks/hooks.json`). The file handling is
  tested: merge, idempotent re-install, and uninstall that removes only our
  entries. What is **not** verified is a real Codex run firing them. Codex records a
  `trusted_hash` per hook under `[hooks.state]` and has a
  `--dangerously-bypass-hook-trust` flag, so a first run may require trust
  approval. Confirming this needs either the developer's real `~/.codex` (which a
  test must not modify) or a `CODEX_HOME` holding credentials.
- **Handoff between two real agents.** Exercised only through the in-process and
  stdio tests, never with two live CLIs.
- **Cursor and Cline.** Wiring is generated and manually checked; no smoke test.

## How to run the smoke tests

Smoke tests are gated behind the `AGENT_SESSION_SMOKE` env var so they only run
where the agent CLIs are installed (CI or a developer machine with them):

```bash
AGENT_SESSION_SMOKE=1 go test ./test/integration/... -run Smoke -v
```

Each smoke test skips gracefully with a clear message when its agent CLI is not
on PATH, so `go test ./...` stays green everywhere.
