package context

import (
	"fmt"
	"strings"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

type renderer struct{}

func NewRenderer() *renderer {
	return &renderer{}
}

// RenderContext produces a budgeted, human-readable context.md (PRD §4.5).
// Lists and strings are truncated to keep the summary small, but every
// truncation is flagged with an explicit hint to fetch the full state, so the
// agent is never silently working with incomplete information.
func (r *renderer) RenderContext(snapshot *entities.Snapshot, budget ports.ContextBudget) (string, error) {
	var b strings.Builder
	truncated := false

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

	budgetedList(&b, "## Completed", snapshot.Progress.Completed, budget.MaxProgress, budget.MaxItemChars, &truncated)
	budgetedList(&b, "## In progress", snapshot.Progress.Pending, budget.MaxProgress, budget.MaxItemChars, &truncated)

	if len(snapshot.Decisions) > 0 {
		b.WriteString("\n## Decisions\n")
		limit := limit(len(snapshot.Decisions), budget.MaxDecisions)
		for _, d := range snapshot.Decisions[:limit] {
			line := d.Decision
			if d.Reason != "" {
				line += " — (" + d.Reason + ")"
			}
			fmt.Fprintf(&b, "- %s\n", truncate(line, budget.MaxItemChars))
		}
		if limit < len(snapshot.Decisions) {
			truncated = true
			fmt.Fprintf(&b, "- … +%d more decisions\n", len(snapshot.Decisions)-limit)
		}
	}

	if len(snapshot.Blockers) > 0 {
		b.WriteString("\n## Blocked\n")
		limit := limit(len(snapshot.Blockers), budget.MaxBlockers)
		for _, bl := range snapshot.Blockers[:limit] {
			fmt.Fprintf(&b, "- %s\n", truncate(bl.Description, budget.MaxItemChars))
		}
		if limit < len(snapshot.Blockers) {
			truncated = true
			fmt.Fprintf(&b, "- … +%d more blockers\n", len(snapshot.Blockers)-limit)
		}
	}

	if snapshot.Tests.Status != "" && snapshot.Tests.Status != "unknown" {
		b.WriteString("\n## Tests\n")
		fmt.Fprintf(&b, "status: %s, failures: %d\n", snapshot.Tests.Status, snapshot.Tests.Failures)
	}

	if len(snapshot.Files.Modified) > 0 {
		b.WriteString("\n## Changed files\n")
		limit := limit(len(snapshot.Files.Modified), budget.MaxFiles)
		for _, f := range snapshot.Files.Modified[:limit] {
			fmt.Fprintf(&b, "- %s\n", truncate(f, budget.MaxItemChars))
		}
		if limit < len(snapshot.Files.Modified) {
			truncated = true
			fmt.Fprintf(&b, "- … +%d more files\n", len(snapshot.Files.Modified)-limit)
		}
	}

	if snapshot.NextAction != "" {
		b.WriteString("\n## Next action\n")
		b.WriteString(snapshot.NextAction + "\n")
	}

	if truncated {
		b.WriteString("\n> Context truncated for brevity — call `context.get depth=full` for the complete state.\n")
	}

	return b.String(), nil
}

func budgetedList(b *strings.Builder, heading string, items []string, maxItems, maxChars int, truncated *bool) {
	if len(items) == 0 {
		return
	}
	b.WriteString("\n" + heading + "\n")
	limit := limit(len(items), maxItems)
	for _, item := range items[:limit] {
		fmt.Fprintf(b, "- %s\n", truncate(item, maxChars))
	}
	if limit < len(items) {
		*truncated = true
		fmt.Fprintf(b, "- … +%d more\n", len(items)-limit)
	}
}

func limit(n, max int) int {
	if max <= 0 || n < max {
		return n
	}
	return max
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
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
