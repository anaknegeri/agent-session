// Package commands provides the universal slash commands for agent-session,
// installed per-agent so users can run /agent-session in any supported agent.
//
// Each command file is a markdown file with a YAML frontmatter `description`.
// Agents differ in directory:
//
//	Claude Code   .claude/commands/*.md   (project) | ~/.claude/commands (user)
//	OpenCode      .opencode/command/*.md  (project) | ~/.config/opencode/commands (user)
//	Cursor        .cursor/commands/*.md
//	Cline         .clinerules/*.md        (project)
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Command is a single slash command file.
type Command struct {
	Name        string // without leading slash and .md suffix
	Description string // YAML frontmatter description
	Prompt      string // body
}

// All returns the universal agent-session commands.
func All() []Command {
	return []Command{
		{
			Name:        "agent-session",
			Description: "Show the current Agent Session state (task, progress, decisions, blockers, next action)",
			Prompt: `Show the current Agent Session state.

Call the agent-session MCP tools:
1. session.get — find the current session
2. context.get depth=full — load the complete session state

Then present a concise summary covering:
- Current task and status
- Progress (completed / in progress)
- Key decisions
- Open blockers
- Next action

Be brief. This is a status snapshot, not a deep dive.`,
		},
		{
			Name:        "agent-session-checkpoint",
			Description: "Create an Agent Session checkpoint with a next action",
			Prompt: `Create an Agent Session checkpoint.

Call the agent-session MCP tools:
1. context.get depth=full — load the current state
2. session.checkpoint — create a checkpoint with next_action describing the
   most useful next step for whoever continues this work

The next_action should be concrete and actionable, e.g. "Fix the X bug in
file.go, then run the test suite".`,
		},
		{
			Name:        "agent-session-record",
			Description: "Record work in Agent Session: a decision, event, and/or checkpoint",
			Prompt: `Record work in Agent Session using session.record.

Call the MCP tool session.record with the details from $ARGUMENTS (or ask what
to record if none given). You can include:
- decision + decision_reason — an architectural decision you made
- event_type — e.g. test.passed, test.failed, command.executed
- next_action + checkpoint=true — set the next step and snapshot state

Keep it concise and structured.`,
		},
	}
}

// FileName returns the on-disk filename (without directory).
func (c Command) FileName() string {
	return c.Name + ".md"
}

// Render produces the markdown content for a command file. The managed marker
// is placed after the frontmatter so parsers (opencode, cursor) that require
// `---` as the first line still work.
func (c Command) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "description: %s\n", c.Description)
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "<!-- agent-session:managed -->\n\n")
	b.WriteString(strings.TrimSpace(c.Prompt))
	b.WriteString("\n")
	return b.String()
}

// isManaged reports whether a file is owned by agent-session (has the managed
// marker we write), so user-created or user-modified files are never touched.
func isManaged(data string) bool {
	return strings.Contains(data, "<!-- agent-session:managed -->")
}

// Install writes all command files into the given directory (created if
// needed) and returns the paths written. It never overwrites a non-empty file
// that isn't ours, and it is idempotent.
func Install(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create commands dir: %w", err)
	}
	var written []string
	for _, c := range All() {
		path := filepath.Join(dir, c.FileName())
		content := c.Render()
		if existing, err := os.ReadFile(path); err == nil {
			// never touch a file we don't own (user-created or heavily modified)
			if !isManaged(string(existing)) {
				continue
			}
			// already up to date
			if string(existing) == content {
				continue
			}
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", path, err)
		}
		written = append(written, path)
	}
	return written, nil
}

// Uninstall removes the agent-session command files from the given directory.
// Files not owned by us are left untouched. Returns the count removed.
func Uninstall(dir string) (int, error) {
	removed := 0
	for _, c := range All() {
		path := filepath.Join(dir, c.FileName())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !isManaged(string(data)) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return removed, fmt.Errorf("remove %s: %w", path, err)
		}
		removed++
	}
	return removed, nil
}
