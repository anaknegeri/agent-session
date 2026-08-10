package context

import (
	"fmt"
	"strings"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

type renderer struct{}

func NewRenderer() *renderer {
	return &renderer{}
}

// RenderContext produces the human-readable context.md (PRD §4.5).
func (r *renderer) RenderContext(snapshot *entities.Snapshot) (string, error) {
	var b strings.Builder

	b.WriteString("# Agent Session")
	if snapshot.Session.Title != "" {
		b.WriteString(" — " + snapshot.Session.Title)
	}
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "**Project:** %s · **Branch:** %s · **Status:** %s\n",
		snapshot.Workspace.Repository, snapshot.Workspace.Branch, snapshot.Session.Status)
	if snapshot.Workspace.Commit != "" {
		fmt.Fprintf(&b, "**Commit:** %s", snapshot.Workspace.Commit)
		if snapshot.Workspace.Dirty {
			b.WriteString(" (dirty)")
		}
		b.WriteString("\n")
	}
	if snapshot.LastAgent != "" {
		fmt.Fprintf(&b, "**Last agent:** %s\n", snapshot.LastAgent)
	}

	if snapshot.Task.Title != "" {
		b.WriteString("\n## Current task\n")
		fmt.Fprintf(&b, "%s — %s\n", snapshot.Task.Title, snapshot.Task.Status)
	}

	if len(snapshot.Progress.Completed) > 0 {
		b.WriteString("\n## Completed\n")
		for _, item := range snapshot.Progress.Completed {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}

	if len(snapshot.Progress.Pending) > 0 {
		b.WriteString("\n## In progress\n")
		for _, item := range snapshot.Progress.Pending {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}

	if len(snapshot.Decisions) > 0 {
		b.WriteString("\n## Decisions\n")
		for _, d := range snapshot.Decisions {
			if d.Reason != "" {
				fmt.Fprintf(&b, "- %s — (%s)\n", d.Decision, d.Reason)
			} else {
				fmt.Fprintf(&b, "- %s\n", d.Decision)
			}
		}
	}

	if len(snapshot.Blockers) > 0 {
		b.WriteString("\n## Blocked\n")
		for _, bl := range snapshot.Blockers {
			fmt.Fprintf(&b, "- %s\n", bl.Description)
		}
	}

	if snapshot.Tests.Status != "" && snapshot.Tests.Status != "unknown" {
		b.WriteString("\n## Tests\n")
		fmt.Fprintf(&b, "status: %s, failures: %d\n", snapshot.Tests.Status, snapshot.Tests.Failures)
	}

	if len(snapshot.Files.Modified) > 0 {
		b.WriteString("\n## Changed files\n")
		for _, f := range snapshot.Files.Modified {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	if snapshot.NextAction != "" {
		b.WriteString("\n## Next action\n")
		b.WriteString(snapshot.NextAction + "\n")
	}

	return b.String(), nil
}

// RenderHandoff produces the deterministic handoff context (PRD §24).
func (r *renderer) RenderHandoff(snapshot *entities.Snapshot, to string) (string, error) {
	var b strings.Builder

	b.WriteString("You are continuing an existing coding session.\n\n")

	if snapshot.Session.Title != "" {
		fmt.Fprintf(&b, "Task:\n%s\n\n", snapshot.Session.Title)
	}
	if snapshot.LastAgent != "" {
		fmt.Fprintf(&b, "Previous agent:\n%s\n\n", snapshot.LastAgent)
	}

	if len(snapshot.Progress.Completed) > 0 {
		b.WriteString("Completed:\n")
		for _, item := range snapshot.Progress.Completed {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}

	if len(snapshot.Decisions) > 0 {
		b.WriteString("Decisions:\n")
		for _, d := range snapshot.Decisions {
			fmt.Fprintf(&b, "- %s\n", d.Decision)
		}
		b.WriteString("\n")
	}

	if len(snapshot.Blockers) > 0 {
		b.WriteString("Current blocker:\n")
		for _, bl := range snapshot.Blockers {
			fmt.Fprintf(&b, "- %s\n", bl.Description)
		}
		b.WriteString("\n")
	}

	if len(snapshot.Files.Modified) > 0 {
		b.WriteString("Changed files:\n")
		for _, f := range snapshot.Files.Modified {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

	if snapshot.NextAction != "" {
		b.WriteString("Next action:\n")
		b.WriteString(snapshot.NextAction + "\n")
	}

	return b.String(), nil
}
