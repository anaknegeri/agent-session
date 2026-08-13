// agent-session:managed — written by `agent-session init`. Edits are overwritten.
//
// pi ships no MCP client, on purpose ("No MCP. Build CLI tools with READMEs, or
// build an extension that adds MCP support"). This extension is therefore the pi
// equivalent of the SessionStart / Stop / PreCompact hooks the other agents get:
// it shells out to the agent-session CLI at the four points where session state
// is worth loading or saving.
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

// An absolute path rather than a PATH lookup: pi is often launched from a GUI
// (editor integrations, ACP hosts) whose environment has neither nvm nor
// Homebrew on PATH, and a bare "agent-session" would silently resolve to
// nothing there. AGENT_SESSION_BIN overrides it after a move or reinstall.
const BIN = process.env.AGENT_SESSION_BIN ?? __AGENT_SESSION_BIN__;

// Session-layer calls are local SQLite work; anything slower than this is stuck,
// and a hung startup hook would block the whole pi session.
const TIMEOUT_MS = 15_000;

// projectRoot walks up for .agent/config.toml so a pi session outside an
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

  async function run(args: string[], cwd: string): Promise<string | null> {
    try {
      const result = await pi.exec(BIN, args, { cwd, timeout: TIMEOUT_MS });
      if (result.code !== 0) {
        return null;
      }
      return result.stdout.trim();
    } catch {
      // A missing or unrunnable binary must not take the session down with it.
      return null;
    }
  }

  pi.on("session_start", async (event, ctx) => {
    // "reload" re-instantiates extensions without changing session state, and
    // the context message injected earlier is already persisted in the session.
    if (event.reason === "reload") return;
    const root = projectRoot(ctx.cwd);
    if (!root) return;
    const text = await run(["resume", "--agent", "pi"], root);
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

  pi.on("session_shutdown", async (event, ctx) => {
    if (event.reason === "reload") return;
    const root = projectRoot(ctx.cwd);
    if (!root) return;
    await run(["checkpoint", "--label", "auto"], root);
  });
}
