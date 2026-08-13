# Agent Session Format v1

The on-disk layout of a project that uses Agent Session. This is the outermost
contract: everything else is reached by finding `.agent/` first.

Held by [`test/contract/format_v1_test.go`](../../test/contract/format_v1_test.go).

## Directory layout

```
<project>/
└── .agent/
    ├── config.toml          # project configuration, carries the format version
    ├── session.db           # SQLite store (plus -wal / -shm while open)
    └── context/
        └── current.md       # last rendered context document (Context Schema v1)
```

`checkpoints/` is reserved by the format — the name is claimed so nothing else
takes it — but v1 does not create or read it: checkpoint state lives in
`session.db`. A reader must tolerate its absence.

A project root is the nearest ancestor directory containing `.agent/`, stopping at
the repository boundary. `AGENT_SESSION_ROOT` overrides the search. Roots are
stored canonicalised (symlinks resolved), so a project registered through
`/var/...` is still found when opened through `/private/var/...`.

The `-wal` and `-shm` files are SQLite's, not ours; they appear and disappear with
open connections and must not be committed. A `.gitignore` for the format is
`.agent/session.db*` — `config.toml` and `context/current.md` are the two files
worth committing if a team wants shared session context.

## Format version

`.agent/config.toml` carries the version in its own section, written on init:

```toml
[format]
version = 1
```

A config without `[format]` is v1 — the field was added when the format was
specified, not when it changed, so directories created before it need no
migration. A config declaring a *higher* version is refused with
`upgrade agent-session`: the layout was written by a build that knows things this
one does not, and writing into it on stale assumptions is worse than stopping.

## Configuration

`config.toml` sections, all optional and all defaulted (see
[`internal/config`](../../internal/config/config.go)):

| Section | Purpose |
| --- | --- |
| `[format]` | the format version above |
| `[project]` | project name |
| `[context]` | context budget: `max_decisions` 5, `max_blockers` 3, `max_files` 8, `max_events` 10, `max_progress` 10, `max_item_chars` 200, `max_total_chars` 4000, `inject_memory` true, `max_memory` 3 |
| `[retention]` | checkpoints kept per kind: manual 50, auto 20, precompact 10, handoff 20 |
| `[sync]` | sync mode; v1 ships `local-only` |

Unknown keys are ignored rather than rejected, so a config written by a newer
build of the same format version still loads.

## Identifiers

Every record ID is `<prefix>_<16 lowercase hex>`. Prefixes are part of the format:
they appear in event payloads, in handoff documents, and in every ID a human
types on the command line.

| Record | Prefix |
| --- | --- |
| project | `proj` |
| session | `sess` |
| agent session | `asess` |
| task | `task` |
| decision | `decision` |
| blocker | `blocker` |
| checkpoint | `chk` |
| event | `evt` |
| artifact | `art` |
| memory / knowledge | `mem` |
| handoff | `handoff` |

One prefix per record kind. Two prefixes for the same kind is a defect in the
contract, not a cosmetic inconsistency — the contract test asserts prefixes from
records the real write paths produced, because a call site can be right in one
service and wrong in another.

## Timestamps

Two encodings, for two different readers.

**On disk:** `2006-01-02T15:04:05.000000000Z07:00` — RFC 3339 with a *fixed*
nine-digit fraction, always UTC. Timestamps live in TEXT columns, so every
`ORDER BY` compares them as strings, and string order has to match chronological
order. `time.RFC3339Nano` trims trailing zeros, which breaks that: a whole second
renders as `10:00:00Z` and sorts *after* the later `10:00:00.5Z`, because `Z` is
greater than `.`. Every "latest" query is affected — the checkpoint behind
`next_action`, restore, retention pruning. The fixed width removes the
possibility.

**In JSON:** plain RFC 3339 (`2026-08-13T10:00:00Z`). It is read by agents and by
anything consuming a checkpoint, never sorted as text, so the extra digits would
only cost tokens.

## Vocabularies

Frozen value sets used across the format:

- session status: `active`, `paused`, `completed`
- task status: `in_progress`, `completed`, `blocked`, `cancelled`
- blocker status: `open`, `resolved`
- checkpoint kind: `manual`, `auto`, `precompact`, `handoff`
- knowledge kind: `project_knowledge`, `architecture`, `solution`, `preference`,
  `skill`
- sync mode: `local-only`

Adding a value is additive. Renaming one is not: retention is applied per
checkpoint kind, and a renamed kind silently loses the limit that was bounding it.
