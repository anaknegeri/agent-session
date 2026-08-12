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
| Codex | ✅ | 🟡 | 🟡 | 🟡 | ⚠ supported by Codex, not wired by us | — |
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

- **Codex hooks.** Codex 0.147.0 does support hooks — its config carries
  `[hooks.state]` entries for `session_start`, `stop`, `user_prompt_submit` and
  `post_tool_use`, and `codex exec` reports them firing. agent-session does not
  wire any of them: the Codex adapter only runs `codex mcp add`. Wiring
  `session_start` and `stop` would give Codex the same auto-resume and
  auto-checkpoint reliability Claude Code has. Untested, and the trust-hash
  mechanism means an installer has to deal with hook trust.
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
