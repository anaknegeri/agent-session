# Agent Compatibility Matrix

Verified capabilities per agent. The matrix is updated as real-agent smoke
tests are added. Rows are the agent-session capabilities; cells are:
✅ verified by automated test · 🟡 partial · ⚠ known gap · ❌ not supported.

| Agent | MCP tools | Context | Checkpoint | Handoff | Hooks | Slash cmds |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| Claude Code | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Codex | ✅ | ✅ | ✅ | ✅ | ⚠ no hook support | — |
| OpenCode | ✅ | ✅ | ✅ | ✅ | ⚠ via instructions | ✅ |
| Cursor | ✅ | 🟡 | 🟡 | 🟡 | ⚠ | ✅ |
| Cline | ✅ | 🟡 | 🟡 | ⚠ | ⚠ | — |

## Legend

- **MCP tools** — the agent can call agent-session MCP tools over stdio.
- **Context** — `context.get` output is usable and nudges surface.
- **Checkpoint** — `session.checkpoint` / hooks produce snapshots.
- **Handoff** — `handoff` context is produced and consumed by the next agent.
- **Hooks** — SessionStart/Stop/PreCompact fire automatically.
- **Slash cmds** — `/agent-session*` commands are available.

## Verified by test

| Test | Agent | What it proves |
|---|---|---|
| `TestRealMCPOverStdio` | any stdio MCP client | full flow over real stdio subprocess |
| `TestClaudeCodeSmoke` (CI) | Claude Code | init → resume → checkpoint via `claude` CLI |
| `TestCodexSmoke` (CI) | Codex | init → resume via `codex` CLI |
| `TestOpenCodeSmoke` (CI) | OpenCode | init → resume via `opencode` CLI |

## How to run the smoke tests

Smoke tests are gated behind the `AGENT_SESSION_SMOKE` env var so they only run
where the agent CLIs are installed (CI or a developer machine with them):

```bash
AGENT_SESSION_SMOKE=1 go test ./test/integration/... -run Smoke -v
```

Each smoke test skips gracefully with a clear message when its agent CLI is not
on PATH, so `go test ./...` stays green everywhere.
