package agent

import (
	"fmt"
	"os"
	"strings"
)

// UniversalInstructions is appended to AGENTS.md so that any agent which reads
// AGENTS.md (opencode, Codex, Cursor, Claude Code, ...) always uses the session
// layer without being prompted (UC-05 auto-resume).
const UniversalInstructions = `## Agent Session (mandatory workflow)

This project uses Agent Session (agent-session) as its session layer. Always follow this workflow:

1. When a session starts, FIRST call the agent-session MCP tools in order:
   - session.get — find the current session
   - context.get — load the current context
   Continue the existing task; do not start from scratch.
2. Record work as you go:
   - task.create / task.update — track the current task
   - decision.create — record architectural decisions with a reason
   - blocker.create — record blockers
   - event.append — record test results (test.failed / test.passed) and file.changed
3. Before finishing (Stop / session end), create a checkpoint:
   - session.checkpoint — include the next_action so the next agent can continue.
4. To keep context small and share learnings across sessions, summarize:
   - call context.summarize, then store the resulting summary with memory.put
     (kind=project_knowledge). This uses your own model — no external LLM API.
`

// EnsureAGENTSMD idempotently appends the Agent Session section to AGENTS.md
// at root. It returns the path written, or "" when nothing changed.
func EnsureAGENTSMD(root string) (string, error) {
	path := root + "/AGENTS.md"
	section := "## Agent Session (mandatory workflow)"
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read AGENTS.md: %w", err)
		}
		content = nil
	}
	if strings.Contains(string(content), section) {
		return "", nil
	}

	header := ""
	if len(content) > 0 {
		header = "\n"
	}
	updated := string(content) + header + UniversalInstructions
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write AGENTS.md: %w", err)
	}
	return path, nil
}
