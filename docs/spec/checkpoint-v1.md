# Checkpoint Schema v1

The snapshot JSON stored inside a checkpoint. This is the one artefact that
outlives the build that wrote it: resuming a session, `session.diff` and `restore`
all read snapshots written weeks earlier, possibly by another version. A rename or
a type change here is not a refactor, it is a break in a stored format.

Held by [`test/contract/checkpoint_v1_test.go`](../../test/contract/checkpoint_v1_test.go).

## The checkpoint

Returned by `session.checkpoint` and by the `session://checkpoint/latest`
resource.

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | `chk_…` |
| `session_id` | string | `sess_…` |
| `task_id` | string | current task at the time, may be empty |
| `kind` | string | `manual` \| `auto` \| `precompact` \| `handoff` |
| `label` | string | agent-authored, may be empty |
| `next_action` | string | agent-authored; what the next agent should do |
| `agent` | string | who wrote it |
| `created_at` | string | RFC 3339 |
| `snapshot` | string | the snapshot document below, as JSON text |

`snapshot` is a JSON *string*, not a nested object. It is opaque to the store and
parsed only on read, which is what lets the schema below be versioned independently
of the row.

Retention is per kind (`[retention]` in `config.toml`), so kind is load-bearing:
an unrecognised kind is unbounded growth.

## The snapshot

```json
{
  "version": 1,
  "session":   { "id": "sess_…", "title": "…", "status": "active" },
  "workspace": { "repository": "agent-session", "branch": "main",
                 "commit": "abc1234", "dirty": true },
  "task":      { "id": "task_…", "title": "…", "status": "in_progress" },
  "progress":  { "completed": ["…"], "pending": ["…"],
                 "tasks": [{ "id": "task_…", "title": "…", "status": "…" }] },
  "decisions": [{ "id": "decision_…", "session_id": "sess_…",
                  "decision": "…", "reason": "…",
                  "agent": "claude", "created_at": "2026-08-13T10:00:00Z" }],
  "files":     { "modified": ["path/to/file.go"] },
  "tests":     { "status": "passed", "failures": 0 },
  "blockers":  [{ "id": "blocker_…", "session_id": "sess_…",
                  "description": "…", "status": "open",
                  "agent": "claude", "created_at": "…", "resolved_at": "…" }],
  "next_action": "…",
  "last_agent":  "claude",
  "nudges":      ["⚠ Open blocker: …"]
}
```

`nudges` is omitted when empty. Every other key is always present; empty means
empty string, zero or an empty array, never absent.

Field notes:

- `version` — the Checkpoint Schema version this snapshot follows. **Absent means
  v1**: the field was added when the schema was specified, not when it changed, so
  checkpoints written before it are readable unchanged.
- `tests.status` — `passed`, `failed` or `unknown`, derived from the most recent
  `test.*` event; empty when the event log could not be read. Both `unknown` and
  empty render as no Tests section at all, rather than as a claim about the test
  suite. `tests.failures` counts `test.failed` events back to the last
  `test.passed`.
- `files.modified` — git's view at snapshot time, not a log of what an agent
  touched.
- `nudges` — session-layer observations (stale checkpoint, unrecorded files, open
  blockers), not agent-authored content. Some of them quote a blocker description,
  so they are still flattened before rendering.
- Everything under `session.title`, `task.title`, `progress.*`, `decisions[]`,
  `blockers[]` and `next_action` is free text written by an agent. See
  [context-v1.md](context-v1.md) for how it must be rendered.

## Version handling

A reader refuses a snapshot whose `version` is higher than it understands:

```
invalid snapshot: checkpoint chk_… uses snapshot schema v2, this build
understands v1 — upgrade agent-session
```

Refusing is the point. Rendering a shape this build does not understand would hand
the agent a context that may be silently wrong, and the failure is reachable in
practice by downgrading the binary in a project a newer one has been writing to.
