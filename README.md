# Agent Session

> **One session. Any coding agent.**

Universal session & handoff layer for AI coding agents.
Switch between **Claude Code, Codex, OpenCode** — and any future agent — without losing context.

Agent Session is not another AI memory database. It is a **local, Git-aware session
layer**: agents are interchangeable workers, and the session is the portable state of the work.

---

## Features

- **Local-first, zero-config** — single binary + SQLite. No PostgreSQL, Redis, Docker, Node or Python.
- **Agent-agnostic** — shared state over transcripts: task, decisions, progress, files, tests, blockers, next action.
- **Git-aware** — git is the source of truth for code; the session stores only state and context.
- **MCP-native** — 16 tools + 6 resources over stdio or streamable-http.
- **Human-readable** — `.agent/context/current.md` and deterministic handoff context, never locked in a proprietary DB.
- **Checkpoint & handoff** — snapshot the work, hand it to another agent deterministically.
- **Always available** — agents spawn the stdio MCP server on demand; optional user-scope registration.
- **Long-term memory** — knowledge store with SQLite FTS5 full-text search; auto-extracts decisions, resolved blockers and completed tasks from sessions (non-LLM).
- **Agent Plugin packaging** — ships as a portable plugin (agent-plugins.org v1.0.0).

---

## Install

### From a release (macOS / Linux)

```bash
curl -fsSL https://install.agent-session.dev | sh        # installs to /usr/local/bin
AS_INSTALL_DIR=~/.local/bin curl -fsSL ... | sh          # user install, no sudo
```

### From source

```bash
make build             # bin/agent-session + bin/agent-session-mcp
make cross-compile     # dist/agent-session-{os}-{arch} (darwin/linux/windows × amd64/arm64)
make install           # copies to ~/.local/bin
```

Requires only: **Go binary + Git**.

### Global availability in Claude Code

Register at **user scope** so `agent-session` appears in `claude mcp list` from every directory:

```bash
agent-session plugin install claude --scope user
```

---

## Quick start

```bash
cd your-project
agent-session init                # one command, like `git init`: init + wire every agent
agent-session start "Implement OAuth2 PKCE"
agent-session status              # progress, blocked, next
agent-session checkpoint --next-action "Fix refresh token rotation"
agent-session handoff codex       # deterministic context for the next agent
agent-session resume --agent opencode
agent-session history             # event log
agent-session context             # print context.md
```

`agent-session init` is the one-command entrypoint per project, modeled after
`git init`: it initializes the project (creates `.agent/`, starts a session) and
wires AGENTS.md plus all agents. It is idempotent — safe to re-run.
`agent-session setup` is an alias. Add `--no-agents` for the session layer only,
or `--only claude|opencode|codex` to wire a single agent.

> **Not a git repo yet?** `init` reminds you to run `git init` first.
> The session works in local-only mode meanwhile, but Agent Session is Git-aware
> and tracks branch/commit/diff once git exists.

---

## CLI reference

| Command | Description |
|---|---|
| `agent-session init` | One-command setup (like `git init`): init + wire agents (`--only`, `--no-agents`). Alias: `setup` |
| `agent-session start [title]` | Start a new session (`-t, --title`) |
| `agent-session status` | Show current session state |
| `agent-session resume` | Resume the latest session (`-a, --agent`) |
| `agent-session checkpoint` | Create a checkpoint (`--label`, `-n, --next-action`) |
| `agent-session handoff <agent>` | Compose handoff context for `claude`/`codex`/`opencode` |
| `agent-session history` | Recent events |
| `agent-session context` | Print context (`-d, --depth summary|recent|full`) |
| `agent-session doctor` | Health check: project, session, store, git |
| `agent-session mcp` | Run the MCP server (`--transport stdio|streamable-http`, `--addr auto|host:port`) |
| `agent-session plugin pack` | Build the Agent Plugin package |
| `agent-session plugin install <agent>` | Wire an agent (`--scope project|user` for claude) |
| `agent-session plugin uninstall <agent>` | Remove the wiring |
| `agent-session setup` | Wire every agent + AGENTS.md for always-on behavior (`--only`) |
| `agent-session memory put <content>` | Store knowledge (`-k kind`) |
| `agent-session memory list` | List knowledge (`-k kind`, `-n limit`) |
| `agent-session memory search <query>` | Full-text search knowledge |
| `agent-session memory promote` | Promote session decisions/blockers/tasks into memory |

---

## Make every agent use Agent Session automatically

One command per project, modeled after `git init` — it initializes if needed and
wires everything (idempotent):

```bash
cd your-project
agent-session init
```

This wires the "always-on" integration (auto-resume, UC-05) in one step:

- **AGENTS.md** — appends a mandatory workflow section read by OpenCode, Codex,
  Cursor and Claude Code: agents first call `session.get` + `context.get`, track
  work with `task.*` / `decision.*` / `blocker.*` / `event.append`, and create a
  checkpoint before finishing.
- **Claude Code** — `.claude/CLAUDE.md` + hooks: `SessionStart` runs
  `agent-session resume` (context injected into the conversation), `Stop` runs
  `agent-session checkpoint`.
- **OpenCode** — `opencode.json` gets `agent.instructions.system` (verified:
  the agent calls the session tools on its own, no prompting needed).
- **Codex** — `codex mcp add agent-session`.

Idempotent — re-running `agent-session setup` is safe.

To make Agent Session visible in **every** Claude Code project, additionally register
at user scope:

```bash
agent-session plugin install claude --scope user
```

---

## MCP

### "Does the MCP server need to be running?"

No. With **stdio** (the default used by the adapters), Claude Code / OpenCode /
Codex **spawn the server automatically** as a subprocess on demand and shut it
down when the session ends. There is no daemon to manage.

With **streamable-http** the server is a persistent daemon. Auto-start templates:

- macOS: `packaging/launchd/com.agent-session.mcp.plist.tmpl`
- Linux: `packaging/systemd/agent-session-mcp.service.tmpl`

The daemon picks a **unique per-project port** (hash of the project root in
`45100–47099`, with free-port fallback):

```bash
agent-session mcp --transport streamable-http        # e.g. http://127.0.0.1:46688/mcp
agent-session-mcp --transport streamable-http --addr auto
```

In a project that is not yet initialized, the server stays connected and tools
return `project not initialized, run agent-session init` instead of a connection failure.

### Tools

`session.get`, `session.checkpoint`, `session.resume`, `context.get`, `context.update`,
`task.create`, `task.get`, `task.update`, `decision.list`, `decision.create`,
`blocker.create`, `blocker.list`, `blocker.resolve`, `event.append`,
`workspace.status`, `workspace.diff`,
`memory.put`, `memory.get`, `memory.search`, `memory.delete`, `memory.promote`

### Resources

`session://current`, `session://context`, `session://decisions`, `session://tasks`,
`session://workspace`, `session://checkpoint/latest`, `memory://recent`

### Canonical events

`session.started`, `task.created`, `task.updated`, `file.changed`, `command.executed`,
`test.started`, `test.failed`, `test.passed`, `decision.created`, `blocker.created`,
`checkpoint.created`, `handoff.created`, `session.completed`

Large event payloads (>8KB) are stored as artifacts and referenced by `artifact_id`.

### Long-term memory (Phase 4)

Memory is separate from session state (PRD §26). The `knowledge` table is a
long-term store with SQLite FTS5 (porter tokenizer) full-text search:

- **Kinds** — `project_knowledge`, `architecture`, `solution`, `preference`, `skill`
- **Manual** — `memory.put` by the agent or `agent-session memory put`
- **Auto-promote** (non-LLM) — `memory.promote` extracts structured session state:
  `decision → architecture`, `resolved blocker → project_knowledge`, `completed task → solution`.
  Idempotent — already-promoted entities are skipped.

---

## Configuration

`.agent/config.toml` — zero-config defaults, human-editable:

```toml
[project]
name = "sakreta"

[storage]
driver = "sqlite"

[session]
auto_checkpoint = true      # checkpoint after task/decision/test events

[git]
enabled = true

[agents.claude]
enabled = true

[agents.codex]
enabled = true

[agents.opencode]
enabled = true

[sync]
mode = "local-only"         # local-only | git-sync | cloud-sync (phase 2)
```

---

## Local data layout

```
project/
└── .agent/
    ├── config.toml           # configuration
    ├── session.db            # SQLite: projects, sessions, tasks, decisions,
    │                         #   blockers, session_events, checkpoints,
    │                         #   agent_sessions, artifacts, knowledge (+FTS)
    └── context/
        └── current.md        # human-readable current state (re-generated)
```

Everything stays local. Add `.agent/` to `.gitignore`, or commit it later for git-sync (phase 2).

---

## Security & privacy

- **Local only by default** — no data leaves the machine.
- A shared session is **untrusted context**: events and checkpoints carry
  `source`, `agent`, `timestamp` so agents treat them as state, not instructions.
- No credentials or secrets are stored by the session layer.

---

## Development

```bash
make build     # build both binaries
make test      # go test ./...
make vet       # go vet ./...
make plugin    # package the Agent Plugin
```

Layout: clean architecture (`Domain → Application → Infrastructure`) with Wire DI.
See `ARCHITECTURE.md` for the full schema, MCP design and development order.

Testing: SQLite `:memory:` unit tests, real-git integration fixtures, in-process
MCP client tests, and a live end-to-end flow (`init → task → checkpoint → handoff → resume`).

---

## Roadmap

**MVP (implemented):** single binary, SQLite, git detection, sessions, events,
checkpoints, context generation, MCP server, Claude/Codex/OpenCode adapters,
handoff, resume, CLI, local-only, Agent Plugin packaging, auto-checkpoint,
always-on setup.

**Phase 4 (implemented):** long-term memory — knowledge store with SQLite FTS5
search, manual `memory.put`, and non-LLM auto-promote of decisions / resolved
blockers / completed tasks into `architecture` / `project_knowledge` / `solution`.

**Phase 2:** cloud sync, multi-device, PostgreSQL, team sessions, web dashboard.

**Phase 3:** LLM-based context summarization (state stays the source of truth).

**Phase 4 remaining (optional):** vector search (needs embeddings), knowledge graph.

---

## License

MIT
