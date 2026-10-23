# Event Schema v1

The append-only event log. Events are read by other agents, by `timeline`, and by
anything auditing what happened in a session, so a type string is a name other
programs match on — and renaming one does not migrate the events already written
under the old name. Nothing removes an event except checkpoint retention, which
removes a `checkpoint.created` together with the checkpoint it announces
(see [Payloads](#payloads)).

Held by [`test/contract/event_v1_test.go`](../../test/contract/event_v1_test.go).

## The event

| Field | JSON | Notes |
| --- | --- | --- |
| id | `id` | `evt_…` |
| session | `session_id` | `sess_…` |
| type | `type` | one of the canonical types below |
| payload | `payload` | JSON text, `{}` when there is nothing to say |
| agent | `agent` | who appended it |
| time | `timestamp` | RFC 3339 |

The column is `created_at` but the JSON key is `timestamp`. That mismatch is
frozen deliberately rather than left to be tidied up into a break later.

`payload` is always present and always parseable JSON: an append carrying no
payload stores `{}`. Rows written before that was enforced hold `''` in the column
and are left as they are rather than migrated — a reader is handed `{}` for them,
so no reader has to parse an empty string as JSON.

## Canonical types

The namespace is closed: appending an unlisted type is rejected. An open namespace
would make this table a description of habit rather than a contract.

| Type | Written by |
| --- | --- |
| `session.started` | init / first attach |
| `session.completed` | session completion |
| `session.resumed` | `session.resume` / `agent-session resume` |
| `task.created` | `task.create` |
| `task.updated` | `task.update` |
| `decision.created` | `decision.create` |
| `blocker.created` | `blocker.create` |
| `blocker.resolved` | `blocker.resolve` |
| `checkpoint.created` | any checkpoint, including auto and precompact |
| `handoff.created` | `handoff` |
| `file.changed` | the sync service, from git |
| `test.started` | agent |
| `test.passed` | agent |
| `test.failed` | agent |
| `command.executed` | agent |

## Payloads

For events the session layer produces itself, these keys are guaranteed present
and non-empty. A reader following a payload back to the row it describes — the
timeline resolving a checkpoint, an audit resolving a handoff — depends on them.

| Type | Required keys |
| --- | --- |
| `task.created` | `task_id` |
| `task.updated` | `task_id`, `status` |
| `decision.created` | `decision_id` |
| `blocker.created` | `blocker_id` |
| `blocker.resolved` | `blocker_id` |
| `checkpoint.created` | `checkpoint_id` |
| `handoff.created` | `handoff_id`, `checkpoint_id`, `from_agent`, `to_agent` |
| `file.changed` | `files` (array of paths) |

`test.started`, `test.passed`, `test.failed`, `command.executed`,
`session.started`, `session.resumed` and `session.completed` carry whatever the
caller passed. v1 promises nothing about their contents beyond being valid JSON
and the artifact rule below — they are agent input, and pretending otherwise would
freeze a shape no writer is holding to.

A `checkpoint_id` always resolves. Checkpoint retention deletes old checkpoints,
and it deletes the `checkpoint.created` event announcing each one with it — the one
exception to the log being append-only, and the reason no payload in the log points
at a checkpoint that is gone. No other event is ever removed.

## Large payloads

A payload over **8192 bytes** is stored as an artifact and the event carries a
reference instead:

```json
{ "artifact_id": "art_…" }
```

A reader that finds `artifact_id` where it expected the payload has to know that
is normal, and at what size it starts, or it will report the payload as lost. The
threshold is part of the contract for that reason.

The reference resolves: the `artifact.get` tool
([mcp-tools-v1.md](mcp-tools-v1.md)) returns the offloaded payload, scoped to the
session that wrote it. An offload that could not be read back would be a payload
discarded with extra steps.

## Trust

Event payloads are agent-authored. A `command.executed` payload is a record that a
command was run, not an instruction to run it; a `test.failed` payload is output,
not a task list. Treat the log as data. See the SECURITY note in the MCP server
instructions ([mcp-tools-v1.md](mcp-tools-v1.md)).
