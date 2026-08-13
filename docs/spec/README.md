# Agent Session contracts

A session is written by one agent and read by another, usually in a different
process and sometimes by a different build of this binary. Six interfaces cross
that boundary, and this directory specifies them:

| Contract | Version | Spec | What it covers |
| --- | --- | --- | --- |
| Agent Session Format | 1 | [format-v1.md](format-v1.md) | the `.agent/` directory layout, ID prefixes, timestamp encodings |
| Checkpoint Schema | 1 | [checkpoint-v1.md](checkpoint-v1.md) | the snapshot JSON stored in a checkpoint |
| Event Schema | 1 | [event-v1.md](event-v1.md) | event types and their payloads |
| Context Schema | 1 | [context-v1.md](context-v1.md) | the rendered context document |
| Handoff Schema | 1 | [handoff-v1.md](handoff-v1.md) | the rendered handoff document and its event |
| MCP Tool Contract | 1 | [mcp-tools-v1.md](mcp-tools-v1.md) | the tool and resource surface, with annotations |

The version numbers live in code as constants in
[`pkg/contract`](../../pkg/contract/contract.go), and every one of them is held to
this directory by the tests in [`test/contract`](../../test/contract). A drift
fails the build instead of surfacing later as an agent misreading a session.

## When a version has to be bumped

**Bump for any change that could make an existing reader wrong.** Renaming or
removing a field, tool, event type or heading. Changing a type, a unit, an
encoding or a default. Narrowing what a value may contain. Changing an MCP tool
annotation, because a client gates on those. Changing the meaning of something
whose name stayed the same — the worst case, since nothing fails loudly.

**No bump for additive change.** A new optional field, a new tool, a new event
type, a new checkpoint kind, a new section at the end of a rendered document.
Wording changes inside a rendered document are additive too: the specs freeze the
structure of context and handoff output, not the prose.

Additive changes still fail the contract tests, on purpose. The baseline is a
literal in the test file, so extending the surface means extending the baseline
and the spec in the same commit as the code. That is the whole mechanism: it
makes the spec impossible to forget rather than merely expected.

## Which contracts carry their version in the data

Only two: **Format** writes `[format] version` into `.agent/config.toml`, and
**Checkpoint** writes `version` into every snapshot. Those are the two a future
build reads back off disk with nobody left to ask what wrote them, so both also
refuse a version higher than they understand rather than guess at the shape.

The other four are observed live. An MCP client lists the tools it is talking to,
so the surface is self-describing. Context and handoff documents are read by a
model, not parsed, so a version string in them would cost tokens and tell the
reader nothing. Event types are their own version — the type string *is* the
name a reader matches on.

For both versioned contracts, **a missing version means v1, not v0**. The field
was added when the format was specified, not when it changed, so no existing
project or checkpoint needs a migration.

## What is deliberately not frozen

- **The export/import format** (`agent-session export` / `import`). It is a
  transfer format between two runs of the same version, not a boundary between
  agents, and freezing it now would lock in a shape that has not been used in
  anger yet. It mints new IDs on import and drops resolved blockers by design.
- **The database schema.** `internal/infrastructure/database/migrations` is
  versioned separately, in the `schema_migrations` table. It is private to this
  binary; the contracts above are what other programs see.
- **Per-record provenance.** Trust is derived from content kind
  (`entities.Trust`), not stored per record. Adding `source`/`origin`/`trust`
  columns is a v2 discussion, not a v1 gap.
- **Prose in rendered documents.** Only headings, labels, order and the trust
  framing are contractual. See [context-v1.md](context-v1.md).
