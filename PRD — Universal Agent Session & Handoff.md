# PRD — Universal Agent Session & Handoff

**Working title:** Agent Session  
**Tagline:** *One session. Any coding agent.*  
**Status:** Product Requirements Draft  
**Target:** Claude Code, Codex, OpenCode  
**Primary mode:** Local-first, zero-config  
**MVP storage:** SQLite  
**Protocol:** MCP  
**Architecture:** Agent-agnostic

---

# 1. Product Overview

Agent Session adalah sebuah **session/state layer** yang memungkinkan developer berpindah antara berbagai AI coding agent tanpa kehilangan konteks pekerjaan.

Contoh:

```text
Claude Code
    ↓
working on OAuth
    ↓
checkpoint
    ↓
Codex
    ↓
continue OAuth
    ↓
checkpoint
    ↓
OpenCode
    ↓
continue
```

Produk tidak berusaha membuat Claude Code, Codex, atau OpenCode menjadi satu agent.

Sebaliknya, produk menyediakan **shared session state** yang dapat dipahami oleh semua agent.

---

# 2. Problem

Saat ini developer dapat menggunakan beberapa coding agent, tetapi setiap agent memiliki session dan context sendiri.

Contoh:

```text
Claude Code
└── Session A

Codex
└── Session B

OpenCode
└── Session C
```

Ketika developer berpindah agent, developer harus menjelaskan kembali:

- apa yang sedang dikerjakan
- keputusan arsitektur
- file yang sudah diubah
- error yang ditemukan
- test yang gagal
- pekerjaan berikutnya
- constraint project
- alasan di balik keputusan sebelumnya

Akibatnya terjadi:

### Context duplication

Agent baru perlu membaca ulang pekerjaan sebelumnya.

### Context loss

Informasi penting dari agent sebelumnya hilang.

### Vendor/session lock-in

Session hanya dapat dilanjutkan oleh agent tertentu.

### Token waste

Developer harus memasukkan ulang context yang sebenarnya sudah diketahui.

### Poor handoff

Tidak ada standar universal untuk menyerahkan pekerjaan dari satu coding agent ke agent lain.

---

# 3. Product Vision

Membuat **universal session layer** sehingga coding agent dapat diperlakukan seperti interchangeable workers.

```text
                  Shared Session
                       │
          ┌────────────┼────────────┐
          │            │            │
       Claude         Codex      OpenCode
          │            │            │
          └────────────┼────────────┘
                       │
                    Git repo
```

Developer seharusnya dapat mengganti agent kapan saja tanpa kehilangan pekerjaan.

---

# 4. Product Principle

## 4.1 Local-first

Produk harus dapat digunakan tanpa:

- PostgreSQL
- Redis
- Docker
- cloud account
- external server

Install → initialize → langsung digunakan.

---

## 4.2 Agent agnostic

Core tidak boleh bergantung pada Claude Code, Codex, atau OpenCode.

```text
Core
 │
 ├── Claude Adapter
 ├── Codex Adapter
 ├── OpenCode Adapter
 └── Future adapters
```

---

## 4.3 Git-aware

Git menjadi source of truth untuk source code.

Agent Session tidak menyimpan seluruh source code.

Yang disimpan adalah state dan context.

---

## 4.4 State over transcript

Produk tidak bertujuan menyatukan seluruh conversation transcript.

Yang dibagikan adalah:

```text
Task
Decision
Progress
Files
Tests
Errors
Blockers
Next Action
Workspace
Events
Checkpoint
```

---

## 4.5 Human-readable

Context penting harus dapat dibaca manusia.

Contoh:

```text
.agent/
├── session.db
├── context.md
├── decisions/
└── checkpoints/
```

Developer tidak boleh terkunci dalam database proprietary.

---

# 5. Target Users

## Primary

Software developer yang menggunakan lebih dari satu AI coding agent.

Contoh:

- Claude Code untuk reasoning
- Codex untuk implementation
- OpenCode untuk local/open-weight models

---

## Secondary

### AI-heavy engineering teams

Tim yang menggunakan beberapa coding agent dalam workflow engineering.

### AI agent developers

Developer yang ingin membuat agent baru yang dapat participar dalam shared session.

### Power users

Developer yang bekerja pada banyak repository dan banyak agent.

---

# 6. Core Use Cases

## UC-01 — Start Session

Developer masuk ke project:

```bash
agent-session init
```

System mendeteksi:

```text
Git repository
Project name
Current branch
Existing session
```

Kemudian membuat:

```text
.agent/
    session.db
    context.md
```

---

# 7. UC-02 — Claude Code bekerja

Claude mulai mengerjakan task:

```text
Implement OAuth2 PKCE
```

Session menyimpan:

```text
Task:
Implement OAuth2 PKCE

Files:
oauth/server.go
oauth/pkce.go

Decision:
Use rotating refresh tokens

Tests:
3 failures

Next:
Fix refresh token rotation
```

---

# 8. UC-03 — Checkpoint

Agent atau user membuat checkpoint:

```bash
agent-session checkpoint
```

System menyimpan snapshot:

```yaml
task: Implement OAuth2 PKCE

status: in_progress

branch: feature/oauth

changed_files:
  - oauth/server.go
  - oauth/pkce.go

decisions:
  - Use rotating refresh tokens

test_status:
  failed: 3

next_action:
  Fix refresh token rotation
```

---

# 9. UC-04 — Handoff

User ingin berpindah ke Codex:

```bash
agent-session handoff codex
```

System membuat handoff context.

Codex mendapatkan:

```text
Project:
Sakreta

Task:
Implement OAuth2 PKCE

Current progress:
Authorization endpoint completed.
PKCE validation completed.

Changed files:
oauth/server.go
oauth/pkce.go

Decision:
Use rotating refresh tokens.

Current blocker:
3 refresh token tests failing.

Next action:
Fix refresh token rotation.
```

---

# 10. UC-05 — Automatic Resume

Idealnya user bahkan tidak perlu menjalankan command handoff.

Ketika Codex masuk ke repository:

```text
Detected Agent Session

Project: Sakreta
Session: OAuth2 implementation

Last agent: Claude Code
Status: In progress

Would you like to resume?
```

User:

```text
Yes
```

Codex menerima context.

---

# 11. UC-06 — OpenCode mengambil alih

Setelah Codex bekerja:

```text
Codex
 ↓
checkpoint
 ↓
Shared Session
 ↓
OpenCode
```

OpenCode membaca state terbaru.

---

# 12. UC-07 — Resume di komputer lain

Developer:

```bash
git clone project
cd project
agent-session init
```

Jika session state disimpan di Git:

```text
.agent/
```

maka context dapat ikut repository.

Untuk privacy-sensitive state, sistem dapat menyediakan mode:

```text
local-only
git-sync
cloud-sync
```

---

# 13. Core Architecture

```text
                    ┌─────────────────────┐
                    │    Agent Session     │
                    │        Core         │
                    └──────────┬──────────┘
                               │
             ┌─────────────────┼─────────────────┐
             │                 │                 │
             ▼                 ▼                 ▼
          Session           Context            Git
          Engine            Engine           Integration
             │                 │                 │
             └─────────────────┼─────────────────┘
                               │
                            Storage
                               │
                         ┌─────┴─────┐
                         │  SQLite   │
                         └───────────┘
                               ▲
                               │
                              MCP
                               │
                ┌──────────────┼──────────────┐
                │              │              │
                ▼              ▼              ▼
           Claude Code       Codex        OpenCode
```

---

# 14. Components

## 14.1 Session Engine

Bertanggung jawab atas:

- create session
- resume session
- checkpoint
- session status
- session lifecycle
- agent handoff

---

## 14.2 Context Engine

Menghasilkan context yang ringkas untuk agent.

Input:

```text
Events
Git diff
Task
Decisions
Tests
Previous checkpoint
```

Output:

```text
Current State
```

---

## 14.3 Event Engine

Menyimpan event penting:

```text
session.started
task.created
task.updated
file.changed
command.executed
test.started
test.failed
test.passed
decision.created
blocker.created
checkpoint.created
handoff.created
session.completed
```

Event tidak harus menyimpan seluruh output command.

Output besar dapat disimpan sebagai artifact/reference.

---

# 15. Session State

Contoh canonical state:

```yaml
session:
  id: sess_123
  project: sakreta
  status: active

workspace:
  repository: sakreta
  branch: feature/oauth
  commit: a82fd12
  dirty: true

task:
  id: task_001
  title: Implement OAuth2 PKCE
  status: in_progress

progress:
  completed:
    - authorization endpoint
    - PKCE validation

  pending:
    - refresh token rotation

decisions:
  - id: decision_001
    decision: Use rotating refresh tokens
    reason: Prevent token replay

files:
  modified:
    - oauth/server.go
    - oauth/pkce.go

tests:
  status: failed
  failures: 3

blockers:
  - Refresh token rotation test failing

next_action:
  Fix refresh token rotation

last_agent:
  type: claude
```

---

# 16. Storage

## MVP — SQLite

Tidak membutuhkan external database.

```text
.agent/
└── session.db
```

Schema awal:

```text
projects
sessions
session_events
tasks
task_events
decisions
checkpoints
agent_sessions
artifacts
```

---

# 17. Storage Abstraction

Core harus menggunakan interface.

```text
Store
 ├── SQLiteStore
 └── PostgresStore
```

Dengan demikian PostgreSQL dapat ditambahkan tanpa mengubah session engine.

---

# 18. MCP Interface

MCP menjadi interface utama agent.

Minimal tools:

```text
session.get
session.checkpoint
session.resume

context.get
context.update

task.get
task.update

decision.list
decision.create

event.append

workspace.status
workspace.diff
```

---

# 19. MCP Resources

Selain tools, MCP resources dapat menyediakan:

```text
session://current
session://context
session://decisions
session://tasks
session://workspace
session://checkpoint/latest
```

Dengan begitu agent dapat membaca context tanpa harus melakukan banyak tool calls.

---

# 20. Agent Adapter

Setiap agent memiliki adapter.

```text
AgentAdapter

ClaudeAdapter
CodexAdapter
OpenCodeAdapter
```

Interface:

```text
detect()
configure()
getSession()
getCapabilities()
install()
uninstall()
```

Adapter tidak boleh mengubah core session model.

---

# 21. Canonical Event Format

Semua agent harus diterjemahkan ke format event universal.

Contoh:

```json
{
  "id": "evt_123",
  "session_id": "sess_123",
  "agent": "claude",
  "type": "test_failed",
  "timestamp": "2026-08-11T04:30:00+07:00",
  "payload": {
    "command": "go test ./...",
    "failure_count": 3
  }
}
```

---

# 22. Git Integration

System membaca:

```text
git status
git diff
git log
git branch
git rev-parse
```

Git menjadi source of truth untuk:

- source code
- branch
- commit
- diff
- merge state

Session layer hanya menyimpan metadata.

---

# 23. Checkpoint

Checkpoint adalah snapshot dari pekerjaan saat ini.

```text
Checkpoint
├── task state
├── workspace state
├── decisions
├── blockers
├── tests
├── current progress
└── next action
```

Checkpoint dapat dibuat:

### Manual

```bash
agent-session checkpoint
```

### Automatic

Setelah:

```text
task completed
test completed
major decision
agent exit
handoff
```

---

# 24. Handoff Protocol

Handoff harus menghasilkan context yang deterministic.

Format:

```yaml
handoff:
  from: claude
  to: codex

  task:
    title: Implement OAuth2 PKCE
    status: in_progress

  completed:
    - Authorization endpoint
    - PKCE validation

  decisions:
    - Rotating refresh tokens

  blockers:
    - Refresh token rotation tests

  changed_files:
    - oauth/server.go
    - oauth/pkce.go

  next_action:
    Fix refresh token rotation
```

---

# 25. Context Budget

Context tidak boleh memasukkan seluruh history.

Default:

```text
Current task
+
Latest checkpoint
+
Relevant decisions
+
Relevant blockers
+
Git diff summary
+
Recent events
```

Jika agent meminta informasi lebih jauh:

```text
context.get
```

baru mengambil detail.

Prinsip:

```text
Progressive Context Loading
```

---

# 26. Memory

Memory **bukan MVP requirement**.

MVP hanya membutuhkan session state.

Kemudian dapat ditambahkan:

```text
Project Memory
User Preferences
Architecture Knowledge
Reusable Solutions
Skills
```

Memory harus terpisah dari session.

```text
Session
   │
   └── temporary state

Memory
   │
   └── long-term knowledge
```

Dengan demikian produk tidak bergantung pada TencentDB-Agent-Memory.

---

# 27. Security

Shared session harus dianggap sebagai **untrusted context**.

Jangan otomatis menganggap semua memory/event sebagai trusted instruction.

Metadata:

```text
source
agent
timestamp
confidence
verified
```

Context yang dimasukkan ke agent harus dibatasi berdasarkan:

- project
- workspace
- session
- user
- trust level

---

# 28. Privacy

Default:

```text
Local only
```

Tidak ada data dikirim ke server.

SQLite berada di komputer developer.

Cloud sync merupakan fitur optional.

---

# 29. CLI

Command utama:

```bash
agent-session init
agent-session status
agent-session start
agent-session resume
agent-session checkpoint
agent-session handoff
agent-session history
agent-session context
agent-session doctor
```

Contoh:

```bash
agent-session status
```

Output:

```text
Project       Sakreta
Session       OAuth2 PKCE
Status        In Progress
Last Agent    Claude Code
Branch        feature/oauth

Progress      70%

Completed
  ✓ Authorization endpoint
  ✓ PKCE validation

Blocked
  ✗ Refresh token rotation

Next
  Fix refresh token rotation
```

---

# 30. Installation

Target utama:

### macOS

```bash
brew install agent-session
```

### Linux

```bash
curl -fsSL https://install.agent-session.dev | sh
```

### Windows

```text
winget install AgentSession
```

Target akhirnya:

```text
single binary
+
SQLite
+
Git
+
MCP
```

Tidak membutuhkan:

```text
Docker
PostgreSQL
Redis
Node.js
Python
```

untuk core.

---

# 31. Configuration

Contoh:

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
```

---

# 32. Local Data Layout

```text
project/
├── .agent/
│   ├── config.toml
│   ├── session.db
│   ├── context/
│   │   └── current.md
│   ├── decisions/
│   └── checkpoints/
│
├── src/
└── ...
```

`.agent/` dapat dimasukkan atau dikecualikan dari Git berdasarkan konfigurasi.

---

# 33. MVP Scope

MVP harus fokus pada:

### Must Have

- [ ] Single binary
- [ ] SQLite
- [ ] Git detection
- [ ] Project/workspace detection
- [ ] Session creation
- [ ] Session state
- [ ] Event log
- [ ] Checkpoint
- [ ] Context generation
- [ ] MCP server
- [ ] Claude Code integration
- [ ] Codex integration
- [ ] OpenCode integration
- [ ] Handoff
- [ ] Resume
- [ ] CLI
- [ ] Local-only mode

### Should Have

- [ ] Automatic checkpoint
- [ ] Git diff integration
- [ ] Test result tracking
- [ ] Decision tracking
- [ ] Blocker tracking
- [ ] Human-readable context.md
- [ ] Session history

### Not MVP

- [ ] Cloud
- [ ] PostgreSQL
- [ ] Team collaboration
- [ ] Vector database
- [ ] Knowledge graph
- [ ] Long-term memory
- [ ] AI-generated persona
- [ ] Billing
- [ ] Web dashboard

---

# 34. Phase 2

Setelah MVP stabil:

```text
Cloud Sync
Multi-device
PostgreSQL
Team Session
Session sharing
Remote session
Web dashboard
```

Architecture:

```text
                    Cloud
                     │
                PostgreSQL
                     │
             ┌───────┴───────┐
             │               │
          Laptop A        Laptop B
          SQLite           SQLite
```

---

# 35. Phase 3 — Intelligent Context

Tambahkan LLM hanya jika diperlukan.

```text
Raw Events
    ↓
Context Extractor
    ↓
Task
Decision
Blocker
Progress
Next Action
```

LLM digunakan untuk **meringkas state**, bukan menjadi source of truth.

---

# 36. Phase 4 — Long-term Memory

Tambahkan:

```text
Project Knowledge
Architecture
Solutions
Preferences
Skills
```

Kemudian optional:

```text
SQLite FTS
+
Vector Search
+
Knowledge Graph
```

---

# 37. Recommended Technology

Bahasa implementasi **tidak dikunci**.

Namun untuk core CLI + local daemon, kandidat utama:

### Go

Kelebihan:

- single binary
- cross-platform
- SQLite support
- excellent CLI ecosystem
- low resource usage
- cocok untuk daemon/MCP

### Rust

Kelebihan:

- excellent single binary
- memory safety
- sangat cocok untuk local infrastructure

### TypeScript

Kelebihan:

- integrasi ecosystem AI/MCP sangat mudah
- development cepat

Kekurangan:

- runtime dependency jika tidak dibundle

### Rekomendasi

**Go untuk core.**

Tetapi desain API harus language-agnostic sehingga agent adapter dapat ditulis dengan bahasa lain.

---

# 38. Success Metrics

MVP dianggap berhasil jika developer dapat melakukan:

```text
Claude Code
     ↓
checkpoint
     ↓
Codex
     ↓
continue
     ↓
checkpoint
     ↓
OpenCode
     ↓
continue
```

tanpa developer harus menjelaskan ulang context secara manual.

Target:

### Context recovery

> ≥ 90% informasi penting dapat dipulihkan.

### Handoff

> Agent berikutnya dapat mulai bekerja dalam ≤ 30 detik setelah resume.

### Installation

> Install → initialize → usable tanpa external database.

### Resource

Local daemon harus memiliki footprint rendah.

---

# 39. Example End-to-End

Developer:

```bash
cd sakreta
agent-session init
```

Claude Code bekerja.

```text
Implement OAuth2 PKCE
```

Setelah beberapa menit:

```bash
agent-session status
```

```text
OAuth2 PKCE

Progress: 70%

✓ Authorization endpoint
✓ PKCE validation

✗ Refresh token rotation

Next:
Fix refresh token rotation
```

Developer berpindah:

```bash
agent-session handoff codex
```

Codex mendapatkan:

```text
You are continuing an existing coding session.

Task:
Implement OAuth2 PKCE.

Previous agent:
Claude Code

Completed:
- Authorization endpoint
- PKCE validation

Decision:
Use rotating refresh tokens.

Current blocker:
Refresh token rotation tests are failing.

Changed files:
- oauth/server.go
- oauth/pkce.go

Next action:
Fix refresh token rotation.
```

Codex menyelesaikan pekerjaan.

Kemudian:

```bash
agent-session handoff opencode
```

OpenCode mendapatkan state terbaru.

---

# 40. Competitive Positioning

Produk tidak diposisikan sebagai:

> Another AI memory database.

Tetapi:

> **Universal session and handoff layer for AI coding agents.**

Kategori:

```text
AI Developer Infrastructure
```

Problem yang dijual:

> **Switch coding agents without losing context.**

Differentiator:

1. Local-first
2. Zero-config
3. Agent-agnostic
4. Git-aware
5. MCP-native
6. SQLite by default
7. Human-readable state
8. Optional cloud
9. Session handoff, bukan hanya memory
10. Open protocol

---

# 41. Product Philosophy

Core abstraction:

```text
Agent ≠ Session
```

Agent hanya worker.

Session adalah state pekerjaan.

```text
             Session
                │
      ┌─────────┼─────────┐
      │         │         │
    Claude     Codex    OpenCode
      │         │         │
      └─────────┼─────────┘
                │
              Git
```

Dengan model ini, agent dapat diganti kapan saja.

---

# 42. Future Vision

Pada akhirnya:

```text
                    Agent Session
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
      Memory            State             Git
        │                 │                 │
      Skills           Tasks            Workspace
      Facts            Events            Changes
      Decisions        Checkpoints       Commits
        │                 │                 │
        └─────────────────┼─────────────────┘
                          │
                     Agent Protocol
                          │
      ┌───────────┬───────┼────────┬───────────┐
      │           │       │        │           │
   Claude       Codex  OpenCode  Cursor     Future
```

Visi akhirnya adalah membuat **session menjadi portable**, bukan agent.

Developer tidak lagi berpikir:

> "Saya sedang memakai Claude Code."

Tetapi:

> "Saya sedang mengerjakan session X, dan Claude/Codex/OpenCode hanyalah agent yang sedang mengerjakannya."

---

# 43. MVP Definition of Done

MVP selesai ketika:

1. Developer dapat meng-install satu binary.
2. Developer dapat menjalankan `agent-session init`.
3. Project otomatis terdeteksi.
4. SQLite otomatis dibuat.
5. Claude Code dapat membaca session.
6. Codex dapat membaca session.
7. OpenCode dapat membaca session.
8. Agent dapat membuat/update checkpoint.
9. Developer dapat melakukan handoff.
10. Agent berikutnya mendapatkan current state.
11. Git branch/diff dapat dibaca.
12. Tidak ada PostgreSQL/Docker/Redis yang diperlukan.
13. Semua data dapat digunakan secara lokal.
14. Session dapat dipulihkan setelah agent ditutup.
15. Tidak diperlukan perubahan pada source code project untuk menggunakan core engine.

---

# 44. Recommended Development Order

```text
Step 1
SQLite schema
      ↓
Step 2
Session engine
      ↓
Step 3
Git integration
      ↓
Step 4
Checkpoint
      ↓
Step 5
MCP server
      ↓
Step 6
Claude adapter
      ↓
Step 7
Codex adapter
      ↓
Step 8
OpenCode adapter
      ↓
Step 9
Handoff protocol
      ↓
Step 10
Automatic context
      ↓
Step 11
CLI UX
      ↓
Step 12
Packaging/installers
```

**Jangan mulai dari AI memory.**

Mulai dari:

```text
Session
+
Event
+
Checkpoint
+
Git
+
MCP
```

Itulah core product.

Memory, vector search, cloud, knowledge graph, dan intelligence dapat ditambahkan setelah masalah **"switch agent tanpa kehilangan pekerjaan"** benar-benar terselesaikan.