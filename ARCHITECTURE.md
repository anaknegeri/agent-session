# Arsitektur MVP — Agent Session

> Sumber: `PRD — Universal Agent Session & Handoff.md`
> Keputusan: **Go** untuk core (single binary + SQLite + MCP + CLI dalam satu executable)
> Packaging: **Agent Plugin** (agent-plugins.org v1.0.0) — MCP server di-ship sebagai stdio subprocess `./bin/agent-session-mcp`

---

## 1. Keputusan Kunci

| Area | Keputusan | Alasan |
|---|---|---|
| Bahasa | Go | single binary, cross-compile 3 OS, footprint rendah, `./bin/server` plugin-relative tanpa runtime dep |
| SQLite | `modernc.org/sqlite` (pure Go, no cgo) | cross-compile mulus ke macOS/Linux/Windows |
| ORM | GORM + `glebarez/sqlite` (pure-Go driver) | konsisten dengan konvensi repo, mudah tambah Postgres nanti |
| MCP | `mark3labs/mcp-go` | stdio + Streamable HTTP, tools + resources, maintenance aktif |
| CLI | `spf13/cobra` | ekosistem CLI standar |
| Git | exec `git` via `GitService` interface | git adalah hard dependency produk (PRD §22); paling robust terhadap versi/config |
| Config | `pelletier/go-toml/v2` | PRD §31 format TOML |
| Logging | stdlib `log/slog` | CLI local-first; nol dependency tambahan, sejalan ethos single-binary |
| DI | `google/wire` | konvensi repo (golang-clean-architecture) |
| ID | prefiks + `google/uuid` | `sess_`, `task_`, `evt_`, `chk_` (PRD §15) |

---

## 2. Layout Repository

```
agent-session/
├── cmd/
│   ├── agent-session/              # entry CLI
│   └── agent-session-mcp/          # entry MCP server (stdio default)
│
├── internal/
│   ├── domain/                     # LAYER: Domain — entities + interfaces, tanpa dependency
│   │   ├── entities/               # Project, Session, Task, Decision, Blocker, Event, Checkpoint, AgentSession, Artifact
│   │   ├── repositories/           # interface repository (per entity)
│   │   └── errors/                 # ErrSessionNotFound, ErrNotInitialized, dst.
│   │
│   ├── application/                # LAYER: Application — use cases / services
│   │   ├── services/               # SessionService, CheckpointService, EventService, ContextService,
│   │   │                           #   HandoffService, TaskService, WorkspaceService, DecisionService
│   │   ├── dto/                    # input/output structs (tag: json, db, validate)
│   │   └── ports/                  # interface antar-layer: Store, GitService, ContextRenderer, AgentDetector
│   │
│   ├── infrastructure/             # LAYER: Infrastructure — implementasi konkret
│   │   ├── database/
│   │   │   ├── sqlite.go           # open/koneksi + pragma (WAL, foreign_keys)
│   │   │   ├── migrations.go       # schema SQL (embedded via embed.FS)
│   │   │   └── store/              # sqlite_store.go + per-entity store implement repository interface
│   │   ├── git/                    # git_runner.go — status/diff/log/branch/rev-parse via exec
│   │   ├── mcp/
│   │   │   ├── server.go           # bootstrap MCP server (stdio | streamable-http)
│   │   │   ├── tools/              # session/context/task/decision/event/workspace tools
│   │   │   └── resources/          # session://current, session://context, dst.
│   │   ├── context/                # markdown_renderer.go — generate context.md / handoff text
│   │   └── agent/                  # adapter.go (interface) + claude/ codex/ opencode/ plugin/
│   │
│   ├── config/                     # config.go — baca .agent/config.toml (PRD §31)
│   ├── providers/                  # constructor Wire provider per layer
│   └── wire/                       # wire.go + wire_gen.go
│
├── cli/                            # command cobra
│   ├── root.go  init.go  status.go  start.go  resume.go
│   ├── checkpoint.go  handoff.go  history.go  context.go
│   ├── doctor.go  mcp.go  plugin.go
│
├── pkg/
│   └── appdir/                     # lokasi dir lintas platform (os.UserConfigDir/UserHomeDir)
│
├── plugin/                         # paket Agent Plugin (di-pack saat build)
│   ├── plugin.json                 # name: agent-session
│   ├── mcp.json                    # stdio → ./bin/agent-session-mcp
│   ├── skills/                     # skill opsional untuk agent
│   └── dev.agentsession/           # extension namespace per-client (hooks/config template)
│
├── scripts/
│   ├── build.sh                    # build CLI + MCP binary
│   ├── cross-compile.sh            # GOOS=darwin/linux/windows × amd64/arm64
│   └── package-plugin.sh           # isi bin/ + zip plugin
│
├── test/
│   ├── fixtures/                   # repo git kecil + project dummy
│   └── integration/                # end-to-end: init → checkpoint → handoff → resume
│
├── Makefile
├── go.mod
└── ARCHITECTURE.md
```

### Aliran dependency (clean architecture)

```
cli/  ─┐
mcp/   ─┤→ application/services ─→ domain/repositories (interface)
        │            │                    │
        └────────────┼────────────────────┘
                     ↓
              infrastructure/
        database/store, git, context, agent
```

Handler MCP & CLI **hanya** inject service (anti-pattern: handler tidak boleh inject repository).

---

## 3. Domain Model & Schema SQLite

### Entities (`internal/domain/entities`)

```go
type Project struct {
    ID        string `json:"id" db:"id"`
    Name      string `json:"name" db:"name" validate:"required"`
    Path      string `json:"path" db:"path"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Session struct {
    ID          string `json:"id" db:"id"`
    ProjectID   string `json:"project_id" db:"project_id"`
    Title       string `json:"title" db:"title"`
    Status      string `json:"status" db:"status"`            // active|paused|completed
    Branch      string `json:"branch" db:"branch"`
    Commit      string `json:"commit,omitempty" db:"commit"`
    Dirty       bool   `json:"dirty" db:"dirty"`
    LastAgent   string `json:"last_agent,omitempty" db:"last_agent"`
    CurrentTaskID string `json:"current_task_id,omitempty" db:"current_task_id"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Task struct {
    ID        string `json:"id" db:"id"`
    SessionID string `json:"session_id" db:"session_id"`
    Title     string `json:"title" db:"title" validate:"required"`
    Status    string `json:"status" db:"status"` // in_progress|completed|blocked|cancelled
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Decision struct {
    ID        string `json:"id" db:"id"`
    SessionID string `json:"session_id" db:"session_id"`
    Decision  string `json:"decision" db:"decision" validate:"required"`
    Reason    string `json:"reason,omitempty" db:"reason"`
    Agent     string `json:"agent,omitempty" db:"agent"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Blocker struct {
    ID          string `json:"id" db:"id"`
    SessionID   string `json:"session_id" db:"session_id"`
    Description string `json:"description" db:"description" validate:"required"`
    Status      string `json:"status" db:"status"` // open|resolved
    Agent       string `json:"agent,omitempty" db:"agent"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    ResolvedAt  *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
}

type SessionEvent struct {
    ID        string    `json:"id" db:"id"`
    SessionID string    `json:"session_id" db:"session_id"`
    Agent     string    `json:"agent" db:"agent"`
    Type      string    `json:"type" db:"type"`
    Payload   JSONB     `json:"payload,omitempty" db:"payload"` // raw JSON string
    CreatedAt time.Time `json:"timestamp" db:"created_at"`
}

type Checkpoint struct {
    ID         string    `json:"id" db:"id"`
    SessionID  string    `json:"session_id" db:"session_id"`
    TaskID     string    `json:"task_id,omitempty" db:"task_id"`
    Label      string    `json:"label,omitempty" db:"label"`
    Snapshot   JSONB     `json:"snapshot" db:"snapshot"` // canonical state (PRD §15)
    NextAction string    `json:"next_action,omitempty" db:"next_action"`
    Agent      string    `json:"agent,omitempty" db:"agent"`
    CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

type AgentSession struct {
    ID           string     `json:"id" db:"id"`
    SessionID    string     `json:"session_id" db:"session_id"`
    Agent        string     `json:"agent" db:"agent"`
    StartedAt    time.Time  `json:"started_at" db:"started_at"`
    EndedAt      *time.Time `json:"ended_at,omitempty" db:"ended_at"`
    CheckpointID string     `json:"checkpoint_id,omitempty" db:"checkpoint_id"`
}

type Artifact struct {
    ID        string `json:"id" db:"id"`
    SessionID string `json:"session_id" db:"session_id"`
    Kind      string `json:"kind" db:"kind"` // test_output|diff|log|...
    Path      string `json:"path,omitempty" db:"path"`
    Content   string `json:"content,omitempty" db:"content"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}
```

### Schema SQLite (`internal/infrastructure/database/migrations.sql`)

```sql
PRAGMA foreign_keys = ON;

CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    branch          TEXT NOT NULL,
    commit          TEXT,
    dirty           INTEGER NOT NULL DEFAULT 0,
    last_agent      TEXT,
    current_task_id TEXT REFERENCES tasks(id),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE tasks (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'in_progress',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE decisions (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    decision   TEXT NOT NULL,
    reason     TEXT,
    agent      TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE blockers (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open',
    agent       TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    resolved_at TEXT
);

CREATE TABLE session_events (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    agent      TEXT NOT NULL,
    type       TEXT NOT NULL,
    payload    TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_session_events_session ON session_events(session_id, created_at);

CREATE TABLE checkpoints (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    task_id     TEXT REFERENCES tasks(id),
    label       TEXT,
    snapshot    TEXT NOT NULL,
    next_action TEXT,
    agent       TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE agent_sessions (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    agent         TEXT NOT NULL,
    started_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    ended_at      TEXT,
    checkpoint_id TEXT REFERENCES checkpoints(id)
);

CREATE TABLE artifacts (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    path       TEXT,
    content    TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
```

Storage dibuat mengikuti PRD §17: `Store` interface + `SQLiteStore` (PostgresStore bisa ditambahkan tanpa menyentuh service).

---

## 4. Application Services (`internal/application/services`)

| Service | Tanggung jawab | Command/MCP tools |
|---|---|---|
| `SessionService` | create/resume/status/handoff/lifecycle, atur `last_agent`, buka+tutup `agent_sessions` | `session.get`, `session.resume`, CLI `start/resume/status` |
| `CheckpointService` | buat snapshot canonical (PRD §15), simpan `checkpoints`, auto-checkpoint, restore | `session.checkpoint`, CLI `checkpoint` |
| `EventService` | append `session_events` (canonical event §14.3), query recent | `event.append`, CLI `history` |
| `TaskService` | create/update task, set `current_task_id` | `task.get`, `task.update` |
| `DecisionService` | list/create decision + blocker | `decision.list`, `decision.create` |
| `ContextService` | render context.md (progressive loading §25) + handoff text (§24) | `context.get`, `context.update`, CLI `context` |
| `WorkspaceService` | baca git status/diff/branch/commit (via GitService) | `workspace.status`, `workspace.diff` |
| `HandoffService` | deterministik: compose canonical state → handoff payload, catat `handoff.created`, update `last_agent` | CLI `handoff <agent>` |

### Alur use case inti

**`init`** → deteksi git repo + nama project → buat `.agent/` + `config.toml` + `session.db` (migrasi) → create Project + Session(active) → event `session.started`.

**`checkpoint`** → kumpulkan: task state, workspace (git status/diff), decisions, blockers, tests, next_action → build snapshot JSON → insert checkpoint → event `checkpoint.created` → tulis `context/current.md`.

**`handoff <agent>`** → ambil latest checkpoint + task + decisions + blockers + git summary → compose handoff YAML deterministik → update `last_agent` → event `handoff.created` → output context untuk agent tujuan.

**`resume`** → restore session + latest checkpoint → render context → set `agent_sessions.started_at` baru → output.

### Canonical event types (PRD §14.3/§21)

```
session.started  task.created     task.updated
file.changed     command.executed
test.started     test.failed      test.passed
decision.created blocker.created  checkpoint.created
handoff.created  session.completed
```

Payload besar → `artifacts` (referensi, bukan isi penuh).

---

## 5. MCP Server (`internal/infrastructure/mcp`)

Bootstrap via `cmd/agent-session-mcp`: deteksi transport dari env `--transport stdio|streamable-http` (default stdio). Runtime: jalankan di cwd project, cari `.agent/session.db` di cwd (walk ke atas untuk nested dir, sampai `.agent` atau git root).

### Tools (PRD §18)

| Tool | Skema input (ringkas) |
|---|---|
| `session.get` | `session_id?` (default current) |
| `session.checkpoint` | `label?`, `next_action?`, `task_status?` |
| `session.resume` | `agent?` |
| `context.get` | `depth?` (summary/recent/full) |
| `context.update` | `field`, `value` (task, next_action, dsb.) |
| `task.create` | `title` |
| `task.get` | `task_id?` |
| `task.update` | `task_id`, `title?`, `status?` |
| `decision.list` | `session_id?` |
| `decision.create` | `decision`, `reason?` |
| `blocker.create` | `description` |
| `blocker.list` | `open_only?` |
| `blocker.resolve` | `blocker_id` |
| `event.append` | `type`, `payload?` (payload besar → artifact) |
| `workspace.status` | — |
| `workspace.diff` | `scope?` (stat/full) |

Auto-checkpoint (config `[session] auto_checkpoint = true`, default on) dibuat setelah `task.create`, `task.update`, `decision.create`, dan `test.passed` (PRD §23).

### Resources (PRD §19)

```
session://current              → ringkasan session aktif
session://context              → context.md terbaru
session://decisions            → daftar decisions
session://tasks                → daftar tasks + status
session://workspace            → branch/commit/dirty/stat
session://checkpoint/latest    → snapshot JSON terakhir
```

Semua resource bisa dibaca agent tanpa tool call berulang → hemat token (PRD §25).

---

## 6. Agent Plugins Packaging (`plugin/` + `internal/infrastructure/agent/plugin`)

Sesuai agent-plugins.org v1.0.0, MCP server di-ship sebagai stdio plugin-relative:

```
plugin/
├── plugin.json
│   {
│     "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
│     "name": "agent-session",
│     "version": "0.1.0",
│     "description": "Universal session & handoff layer for AI coding agents",
│     "license": "MIT",
│     "extensions": {
│       "dev.agentsession": {
│         "agent_adapters": ["claude", "codex", "opencode"]
│       }
│     }
│   }
│
├── mcp.json
│   {
│     "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
│     "mcpServers": {
│       "agent-session": {
│         "type": "stdio",
│         "command": "./bin/agent-session-mcp",
│         "cwd": "${PLUGIN_ROOT}",
│         "env": {
│           "AGENT_SESSION_DATA": "${PLUGIN_DATA}"
│         }
│       }
│     }
│   }
│
├── bin/agent-session-mcp        # hasil build Go (plugin-relative, satu token)
├── skills/                      # (opsional) skill ringkas: agent-session/handoff
└── dev.agentsession/            # template hook/config per client:
    ├── claude/  .claude/settings.json (hook Stop/PreToolUse)
    ├── codex/   ~/.codex/config.toml  (mcp add)
    └── opencode/ opencode.json        (mcp server)
```

**Kenapa ini penting:** `command: "./bin/agent-session-mcp"` adalah executable self-contained tanpa runtime host — memenuhi PRD §30 (tanpa Node/Python/Docker) dan Agent Plugins spec §7.2.1 (command single token, no placeholder expansion).

### Adapter (PRD §20) — `internal/infrastructure/agent/adapter.go`

```go
type AgentAdapter interface {
    Detect(ctx context.Context) (bool, error)
    Configure(ctx context.Context, cfg *config.Config) error
    GetSession(ctx context.Context) (*entities.Session, error)
    GetCapabilities() []Capability
    Install(ctx context.Context) error
    Uninstall(ctx context.Context) error
}
```

MVP implementasi:
- **claude** — tulis `.mcp.json` / `~/.claude.json` (mcpServers), hook via `dev.agentsession/claude`
- **codex** — `~/.codex/config.toml` (`[mcp_servers.agent-session]`)
- **opencode** — `opencode.json` (`mcp`)
- **plugin** — installer generic Agent Plugin ke lokasi client (`~/.agents/plugins` atau registry client) → jalur "satu paket untuk semua client"

CLI `agent-session plugin install|uninstall` memanggil adapter di atas.

---

## 7. Context & Handoff Format

### `context.md` (human-readable, PRD §4.5)

Dihasilkan `ContextService` + `markdown_renderer`:

```markdown
# Agent Session — <title>

**Project:** <name> · **Branch:** <branch> · **Status:** <status>
**Last agent:** <last_agent> · **Updated:** <timestamp>

## Current task
<task title> — <status>

## Completed
- <item>

## Blocked
- <blocker>

## Decisions
- <decision> — (reason)

## Tests
<status>, <failures> failed

## Changed files
- <path>

## Next action
<next_action>
```

### Handoff payload (deterministik, PRD §24)

`HandoffService` menyusun YAML dari canonical state — output CLI ditampilkan dan/atau ditulis ke `.agent/handoffs/<from>-to-<to>-<ts>.md`. Format sama persis PRD §24.

### Progressive loading (PRD §25)

`context.get depth=summary` → current task + latest checkpoint + decisions + blockers + git summary + recent events (≤ N). `depth=recent|full` baru mengambil detail/artifacts.

---

## 8. CLI (`cli/` + `cmd/agent-session`)

| Command | Fungsi | Mapping PRD |
|---|---|---|
| `agent-session init` | deteksi project, buat `.agent/` + db + session | UC-01 |
| `agent-session status` | tampilkan state (format PRD §29) | UC contoh |
| `agent-session start` | mulai/buat session baru | — |
| `agent-session resume` | resume session terakhir / by id | UC-05 |
| `agent-session checkpoint` | snapshot manual | UC-03 |
| `agent-session handoff <agent>` | compose handoff context | UC-04 |
| `agent-session history` | daftar event | — |
| `agent-session context` | print/regen context.md | — |
| `agent-session doctor` | cek git, sqlite, config, plugin | — |
| `agent-session mcp` | jalankan MCP server (stdio) | — |
| `agent-session plugin install|uninstall|pack` | kelola Agent Plugin | §20 |

Format output `status` mengikuti contoh PRD §29 (progres, completed ✓, blocked ✗, next).

---

## 9. Konfigurasi (`.agent/config.toml`)

```toml
[project]
name = "sakreta"

[storage]
driver = "sqlite"

[session]
auto_checkpoint = true

[git]
enabled = true

[agents.claude]
enabled = true

[agents.codex]
enabled = true

[agents.opencode]
enabled = true

[sync]
mode = "local-only"   # local-only | git-sync | cloud-sync (phase 2)
```

---

## 10. Urutan Pengembangan (per PRD §44, diterjemahkan ke tasks)

| Step | Deliverable | Verifikasi |
|---|---|---|
| 1 | Schema SQLite + migrasi + `Store` interface + `SQLiteStore` | unit test in-memory SQLite |
| 2 | `SessionService` + repositori (session/task/decision/blocker/event/checkpoint/agentsession) | test lifecycle session |
| 3 | `GitService` (status/diff/log/branch/rev-parse via exec) + `WorkspaceService` | test pakai fixture git repo |
| 4 | `CheckpointService` + canonical snapshot + auto-checkpoint | test snapshot roundtrip |
| 5 | MCP server + tools + resources (mark3labs/mcp-go) | test protocol-level (mcp-go client harness) |
| 6 | Adapter claude | integrasi + `doctor` |
| 7 | Adapter codex | integrasi + `doctor` |
| 8 | Adapter opencode | integrasi + `doctor` |
| 9 | `HandoffService` (deterministik) + CLI `handoff` | test handoff YAML golden file |
| 10 | `ContextService` + progressive loading + auto-context | test konten context.md |
| 11 | CLI UX lengkap + format `status` | snapshot test output |
| 12 | Packaging: `scripts/cross-compile.sh`, `scripts/package-plugin.sh`, installer | smoke test di 3 OS |

Cross-cutting sejak step 1: **Wire DI**, **slog**, config loader, `pkg/appdir`.

---

## 11. Strategi Testing

- **Unit (table-driven):** service + repository pakai `:memory:` SQLite (modernc mendukung), tanpa network.
- **Git:** fixture repo kecil dibuat di test (mkdir + `git init` + commit), jalankan `GitService` terhadapnya.
- **MCP:** gunakan harness client dari `mark3labs/mcp-go` untuk memanggil tools/resources dan assert response.
- **Integration (`test/integration`):** skenario end-to-end `init → task → checkpoint → handoff → resume` + `doctor`.
- **Golden file:** output `handoff` YAML & `context.md` di-lock agar deterministic.

---

## 12. Mapping Definition of Done (PRD §43)

| PRD DoD | Di mana dipenuhi |
|---|---|
| 1. Satu binary | `cmd/agent-session` (CLI + MCP via subcommand) |
| 2. `init` | CLI `init` |
| 3–4. Project detection + SQLite otomatis | `InitService` + migrasi |
| 5–7. Claude/Codex/OpenCode baca session | Adapter + MCP tools/resources |
| 8. Agent buat/update checkpoint | MCP `session.checkpoint` |
| 9–10. Handoff + current state | `HandoffService` + `context.get` |
| 11. Git branch/diff terbaca | `WorkspaceService` + MCP `workspace.*` |
| 12. Tanpa PostgreSQL/Docker/Redis | stack: modernc SQLite + exec git saja |
| 13. Data lokal | `local-only` default |
| 14. Session recover setelah agent tutup | `resume` restore dari checkpoint |
| 15. Tanpa ubah source project | semua state di `.agent/` |
