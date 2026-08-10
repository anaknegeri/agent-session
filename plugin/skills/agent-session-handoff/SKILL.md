---
name: agent-session-handoff
description: Record a checkpoint and produce a handoff context for the next coding agent.
---

Use this skill when switching coding agents. It summarizes the current session
so another agent can continue without losing context.

1. Call `session.checkpoint` with a `label` and `next_action`.
2. Call `context.get` with `depth=summary` to read the current state.
3. Present the summary so the next agent can resume.
