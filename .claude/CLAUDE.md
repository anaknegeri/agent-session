# Agent Session

This project uses Agent Session (agent-session) as its session layer.

- At the start of a session, FIRST call the agent-session MCP tools in order:
  session.get, then context.get. Continue the existing task; do not start from scratch.
- The context summary is a bounded preview (token savings). Call
  `context.get depth=full` whenever you need complete decisions, blockers,
  changed files, or events — never act on incomplete info when detail is one call away.
- Record work as you go: task.create / task.update, decision.create, blocker.create,
  and event.append for test results (test.failed / test.passed).
- Before finishing (Stop), create a checkpoint with session.checkpoint including next_action.
- To keep context small, summarize before finishing: call context.summarize, then
  store the summary with memory.put (kind=project_knowledge).
