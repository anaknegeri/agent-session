// agent-session:managed — written by `agent-session init`. Edits are overwritten.
//
// omp has an MCP client, so the model can read and record session state with the
// agent-session MCP tools on its own. This extension covers what a tool call
// cannot guarantee:
//
//   - session_start / before_agent_start — the session state is in front of the
//     model on turn one, whether or not it thinks to ask for it
//   - session_before_compact — a checkpoint exists before the conversation is
//     summarized away
//   - session_stop — a checkpoint exists when a turn settles. This is omp's
//     equivalent of Claude Code's Stop hook, and the checkpoint that reliably
//     lands; it never fires for a task/subagent session
//   - session_shutdown — last resort for a session that ends without settling a
//     turn, under omp's hard 2s teardown budget
//
// Everything runs through the agent-session CLI rather than the MCP server: hooks
// fire outside a turn, where no tool call can be issued.
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";

// An absolute path rather than a PATH lookup: omp is often launched from a GUI
// (editor integrations, ACP hosts) whose environment has neither nvm nor
// Homebrew on PATH, and a bare "agent-session" would silently resolve to
// nothing there. AGENT_SESSION_BIN overrides it after a move or reinstall.
const BIN = process.env.AGENT_SESSION_BIN ?? __AGENT_SESSION_BIN__;

// Session-layer calls are local SQLite work; anything slower than this is stuck,
// and a hung startup hook would block the whole omp session.
const TIMEOUT_MS = 15_000;

// omp caps session_shutdown handlers at 2s (SESSION_SHUTDOWN_HANDLER_TIMEOUT_MS)
// so teardown stays prompt, and abandons the handler after it. A checkpoint that
// asked for the generic budget there would be cut off mid-flight on any repo
// where git is slow, which is why the shutdown call gets a budget that fits and
// session_stop carries the checkpoint that matters.
const SHUTDOWN_TIMEOUT_MS = 1_500;

// Module state, deliberately not per-session: omp imports this module once per
// process and hands the same instance to every session runner it creates —
// including one per `task` subagent, which inherits the parent's extension paths.
// `resume` closes and reopens the session's agent_session row, so resuming per
// runner would churn that row once per subagent and prepend the whole project
// context to prompts that were deliberately scoped.
let resumed = false;

// projectRoot walks up for .agent/config.toml so an omp session outside an
// agent-session project costs nothing. The CLI does the same search, but doing
// it here keeps us from spawning a process per session in every other project.
function projectRoot(start: string): string | null {
  let dir = resolve(start);
  for (;;) {
    if (existsSync(join(dir, ".agent", "config.toml"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

export default function (pi: ExtensionAPI) {
  // Context loaded at session_start, handed to the LLM on the first turn.
  let pendingContext: string | null = null;

  async function run(
    args: string[],
    cwd: string,
    options: { timeout?: number; signal?: AbortSignal } = {},
  ): Promise<string | null> {
    try {
      const result = await pi.exec(BIN, args, {
        cwd,
        timeout: options.timeout ?? TIMEOUT_MS,
        signal: options.signal,
      });
      if (result.code !== 0) {
        return null;
      }
      return result.stdout.trim();
    } catch {
      // A missing or unrunnable binary must not take the session down with it.
      return null;
    }
  }

  pi.on("session_start", async (_event, ctx) => {
    if (resumed) return;
    const root = projectRoot(ctx.cwd);
    if (!root) return;
    // Set before the await: two runners starting concurrently must still resume once.
    resumed = true;
    const text = await run(["resume", "--agent", "omp"], root);
    if (text === null) {
      // An extension that looks installed but records nothing is worse than an
      // error, so a broken wiring is said out loud once, at startup.
      if (ctx.hasUI) {
        ctx.ui.notify(`agent-session: cannot run ${BIN} — session state is not being recorded`, "warning");
      }
      return;
    }
    pendingContext = text;
  });

  pi.on("before_agent_start", async (_event, _ctx) => {
    if (!pendingContext) return;
    const content = pendingContext;
    pendingContext = null;
    return {
      message: {
        customType: "agent-session",
        content,
        display: true,
      },
    };
  });

  pi.on("session_before_compact", async (_event, ctx) => {
    const root = projectRoot(ctx.cwd);
    if (!root) return;
    await run(["checkpoint", "--label", "precompact"], root);
  });

  pi.on("session_stop", async (event, ctx) => {
    const root = projectRoot(ctx.cwd);
    if (!root) return;
    // The signal cancels the checkpoint when the settle pass is aborted, so a
    // Ctrl+C never waits on it. Returning nothing lets the turn settle.
    await run(["checkpoint", "--label", "auto"], root, { signal: event.signal });
  });

  pi.on("session_shutdown", async (_event, ctx) => {
    const root = projectRoot(ctx.cwd);
    if (!root) return;
    await run(["checkpoint", "--label", "auto"], root, { timeout: SHUTDOWN_TIMEOUT_MS });
  });
}
