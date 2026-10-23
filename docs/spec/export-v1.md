# Export Schema v1

The document `agent-session export` writes and `agent-session import` reads back.
Unlike a checkpoint's snapshot, an export is handed to a person — attached to a
ticket, dropped in a chat, committed to a repo — and may be imported by a build
far newer or older than the one that wrote it. It crosses exactly the boundary a
checkpoint does: exported with 0.1.8, imported with 0.2.x.

Held by [`test/contract/export_v1_test.go`](../../test/contract/export_v1_test.go).

## The document

```json
{
  "version": 1,
  "session": {
    "id": "sess_…", "project_id": "proj_…",
    "title": "…", "status": "active",
    "branch": "main", "commit": "abc1234", "dirty": true,
    "last_agent": "claude", "current_task_id": "task_…",
    "created_at": "2026-08-13T10:00:00Z", "updated_at": "2026-08-13T10:05:00Z"
  },
  "tasks": [{ "id": "task_…", "session_id": "sess_…",
              "title": "…", "status": "in_progress",
              "created_at": "…", "updated_at": "…" }],
  "decisions": [{ "id": "decision_…", "session_id": "sess_…",
                  "decision": "…", "reason": "…",
                  "agent": "claude", "created_at": "…" }],
  "blockers": [{ "id": "blocker_…", "session_id": "sess_…",
                 "description": "…", "status": "open",
                 "agent": "claude", "created_at": "…", "resolved_at": "…" }],
  "events": [{ "id": "evt_…", "session_id": "sess_…", "agent": "claude",
               "type": "task.created", "payload": "{…}", "timestamp": "…" }],
  "checkpoints": [{ "id": "chk_…", "session_id": "sess_…", "task_id": "…",
                    "kind": "manual", "label": "…", "snapshot": "{…}",
                    "next_action": "…", "agent": "claude", "created_at": "…" }],
  "memory": [{ "id": "mem_…", "session_id": "sess_…", "kind": "solution",
               "content": "…", "created_at": "…" }],
  "exported_at": "2026-08-13T10:06:00Z"
}
```

Every collection is the row shape of its record kind — `session` is an
[`entities.Session`](../../internal/domain/entities/session.go), `tasks[]` are
[`entities.Task`](../../internal/domain/entities/task.go) rows, and so on for
decisions, blockers, events and checkpoints. `checkpoints[].snapshot` is the
Checkpoint Schema v1 JSON described in [checkpoint-v1.md](checkpoint-v1.md), still
encoded as a string inside this document, exactly as it is stored.

## What is populated

All seven collections are populated by `Export` — none is a placeholder kept
empty by design — but not all of them carry the session's full history:

| Field | Populated with | Cap |
| --- | --- | --- |
| `session` | the session row itself | — |
| `tasks` | every task of the session, oldest first | none |
| `decisions` | every decision of the session, oldest first | none |
| `blockers` | every blocker of the session, **open and resolved**, oldest first | none |
| `events` | the session's events, **newest first** | 500 |
| `checkpoints` | the session's checkpoints, **newest first** | 500 |
| `memory` | the session's knowledge entries, **newest first** | 500 |
| `exported_at` | the moment `Export` ran, not any record's own timestamp | — |

`tasks`, `decisions` and `blockers` have no cap and no ordering surprise: an
export always carries every one of them. `events`, `checkpoints` and `memory`
are capped at 500 rows each (`exportLimit` in `export_service.go`) and ordered
newest-first, so a session with a long history exports its *most recent* 500
events and checkpoints, not its earliest — the ones still in the database beyond
that cap are simply not in the file. This is a deliberate size bound ("one
runaway session cannot produce a file no reader can hold"), not a bug, but a
reader treating `events`/`checkpoints`/`memory` as the complete log of a session
older than 500 entries in any of them is wrong to.

All seven collections are `[]` when empty, **never `null`** — verified for a
session with zero tasks, decisions, blockers, events, checkpoints and memory
entries; the JSON marshaller does not have to special-case an empty session.

**Trust:** `ExportJSON` marshals the collected rows unflattened — a task title,
decision, reason, blocker description or memory entry that carries newlines
reaches the JSON document exactly as it was recorded. `ExportMarkdown` flattens
every one of those fields with `safetext.SingleLine` before rendering, and marks
the sections that hold them (see the SECURITY note in
[mcp-tools-v1.md](mcp-tools-v1.md)). A reader parsing the JSON document itself —
rather than going through `ExportMarkdown` — is holding agent-authored free text,
not a trusted transcript.

## Version handling

Absent `version` means v1: the field was added when this schema was specified,
not when it changed, so export files already on disk need no migration —
`Import` treats a missing or zero `version` as `1` and proceeds. `Import`
refuses a document from a newer schema:

```
export file uses export schema v2, this build understands v1 — upgrade agent-session
```

Refusing is the point, for the same reason `ParseSnapshot` refuses a newer
checkpoint: a future version may have changed what a field means, and importing
it on this build's assumptions is worse than stopping.

Unknown top-level keys are ignored, not rejected: `json.Unmarshal` into the
document shape above drops anything it has no field for, so a document written
by a newer build with an extra top-level key still imports on an older one.

## What Import drops

`Import` recreates a session tree from the document; it does not restore one.
Every record it writes gets a freshly minted ID and a store-assigned
`created_at`/`updated_at` — the document's own IDs and timestamps are never
written anywhere. Specifically:

- **`session`** — a new session is created in the target project (which need not
  be the one it was exported from). `status` is **forced to `active`**
  regardless of what the document held (`active`, `paused` and `completed` all
  become `active`). `branch`, `commit`, `dirty`, `last_agent` and
  `current_task_id` from the document are **ignored entirely**: the new session
  gets `last_agent` set to the *importing* agent, and `current_task_id` ends up
  pointing at whichever imported task was created last (or stays empty when
  there are none) — never the value recorded in the document. `title` is kept,
  flattened to a single line, and has `" (imported)"` appended (falling back to
  `"Imported session"` when the flattened title is empty).
- **`tasks`** — every task is recreated, but `status` is **forced to
  `in_progress`** no matter what the document says: a `completed`, `blocked` or
  `cancelled` task imports as `in_progress` every time.
- **`decisions`** — every decision is recreated with `decision` and `reason`
  flattened; `agent` is **replaced with the importing agent** — the document's
  own per-decision `agent` is read but never written.
- **`blockers`** — only blockers whose `status` is `open` are imported at all;
  every `resolved` blocker, and its `resolved_at`, is **silently dropped**. The
  ones that are imported get `description` flattened and `agent` replaced with
  the importing agent, the same as decisions.
- **`events`, `checkpoints`, `memory`** — **none of the three is read by
  `Import`.** A document's event history, its checkpoints (including every
  stored `snapshot`), and its knowledge entries have no effect on an import:
  they are not recreated, not referenced, not validated. The only event an
  import ever writes is one fresh `session.started` for the session it creates.

An import that fails partway leaves nothing behind: the session, its tasks,
decisions and blockers, and the `session.started` event are written in one
transaction.
