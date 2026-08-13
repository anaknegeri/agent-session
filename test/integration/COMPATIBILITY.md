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
| Claude Code | ✅ | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Codex | ✅ | ✅ read-only path | 🟡 | ✅ | 🟡 registered, needs one-time trust | — |
| OpenCode | 🟡 | 🟡 | 🟡 | 🟡 | ⚠ via instructions only | 🟡 |
| Cursor | 🟡 | 🟡 | 🟡 | 🟡 | ⚠ via rules only | 🟡 |
| Cline | 🟡 | 🟡 | 🟡 | ⚠ | ⚠ via rules only | — |

## Legend

- **MCP tools** — the agent can call agent-session MCP tools over stdio.
- **Context** — `context.get` output is usable and nudges surface. A sandboxed
  client may only reach `context.read` (see the Codex note below); the cell says
  which path is covered.
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
| `TestCrossAgentHandoffSmoke` | Claude Code → Codex | a marker task Claude records through MCP survives `handoff codex` in the same session, and Codex — a separate CLI, separate process, its own MCP server — reports that task back | yes |
| `TestReadOnlyToolsDoNotWrite` | any MCP client | every tool advertising `readOnlyHint` leaves the session fingerprint (session, last agent, current task, event/checkpoint/task/decision/memory counts) untouched | no |

### Attribution caveat for the Claude smoke test

Claude merges user-scope and project-scope settings. On a machine where
`agent-session init` has already wired `~/.claude/settings.json`, those hooks fire
in any project containing `.agent/`, so a passing behavioural assertion does not
prove the project-scope wiring works. The file-level assertions are
scope-attributable; the behavioural ones are not, and the test logs a note when it
detects user-scope wiring. Run on a clean machine or in CI to exercise the
project-scope path.

### Codex only executes MCP tools annotated read-only

Under `approval: never` — which is what `codex exec` uses — Codex runs MCP tools
carrying `readOnlyHint` and raises a permission request for every other tool. A
non-interactive run has nobody to answer it, so the request is auto-cancelled and
the tool call comes back `failed` / `user cancelled MCP tool call`.

That made the documented first step unreachable there. `context.get` syncs file
changes and may auto-checkpoint, so it is honestly not read-only and Codex was
right to refuse it — the agent then answered from `session.get` alone and reported
the session title instead of the current task.

`context.read` exists for this: same rendering, no file-change sync and no
auto-checkpoint, annotated read-only. `TestCrossAgentHandoffSmoke` drives Codex
through it under `-s read-only` and asserts the marker comes back, so the claim is
tested rather than assumed. Prefer `context.get` wherever writes are allowed —
file changes stay recorded that way.

The related defect this surfaced: `context.summarize` was annotated read-only
while calling the `context.get` path, so a client trusting the annotation was
writing to the session. `TestReadOnlyToolsDoNotWrite` now holds every read-only
tool to its hint by state fingerprint, which covers tools added later without
touching the test.

Verified on 12 August 2026, macOS, against the real CLIs: `TestClaudeCodeSmoke`
(Claude Code), `TestOpenCodeSmoke` (OpenCode 1.18.15) and `TestCodexSmoke`
(codex-cli 0.147.0) all pass. The Claude run had user-scope wiring present, so
its behavioural half carries the caveat above. The Codex run needed one retry:
the first attempt hit an upstream `Selected model is at capacity` error and
skipped, which is the intended behaviour of the skip path.

`TestCrossAgentHandoffSmoke` passed on 13 August 2026 (Claude Code → codex-cli
0.147.0, 32s). Codex keeps its MCP registration at user scope only, so this test
reads the developer's real `~/.codex`: it never writes there, and it skips when
the registration is missing. It also skips when the *installed* binary Codex is
configured to launch does not yet expose `context.read`, since that would fail the
agent for a capability the server never advertised — the run above needed the
working tree's build installed to exercise it.

### Not covered

- **Codex hooks firing.** `init --only codex` writes `SessionStart` and `Stop`
  hooks into `$CODEX_HOME/hooks.json`, using the same schema Codex plugins use.
  The file handling is tested: merge, idempotent re-install, and uninstall that
  removes only our entries.

  Registration is confirmed against codex-cli 0.147.0 on the real `~/.codex`:
  `hooks/list` on the app-server returns both entries with `source: "user"`,
  `sourcePath: ~/.codex/hooks.json`, `enabled: true`, keys
  `~/.codex/hooks.json:session_start:0:0` and `:stop:0:0`. So the path and the
  schema are right.

  They report `trustStatus: "untrusted"` and therefore do **not** run: Codex only
  executes a hook after it is approved once, recording `trusted_hash` under
  `[hooks.state."<key>"]` in `config.toml`. Approval happens in the Codex TUI
  (its hooks review screen); `codex exec` cannot prompt, so a non-interactive run
  silently skips them. Two `codex exec` runs confirmed this — a probe script wired
  through `hooks.json`, and the same script passed as `-c hooks.SessionStart=...`,
  neither of which executed, while the already-trusted `warp` plugin's hooks did.

  agent-session deliberately does not write `trusted_hash` itself: that is the
  approval gate protecting the user from an installer wiring shell commands into
  their agent. `agent-session init` prints the one-time approval step instead.
  What remains unverified is the post-approval run.
- **Cursor and Cline.** Wiring is generated and manually checked; no smoke test.

## How to run the smoke tests

Smoke tests are gated behind the `AGENT_SESSION_SMOKE` env var so they only run
where the agent CLIs are installed (CI or a developer machine with them):

```bash
AGENT_SESSION_SMOKE=1 go test ./test/integration/... -run Smoke -v
```

Each smoke test skips gracefully with a clear message when its agent CLI is not
on PATH, so `go test ./...` stays green everywhere.
