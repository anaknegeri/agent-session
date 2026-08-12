package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	appsvc "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [before-id] [after-id]",
		Short: "Show what changed between two checkpoints",
		Long: `Show what changed between two checkpoints.

With no arguments: diff the two latest checkpoints.
With one argument:  diff that checkpoint against the latest.
With two arguments:  diff before-id against after-id.`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := open()
			session, err := currentSession(a)
			if err != nil {
				return err
			}

			beforeID, afterID, err := resolveDiffTargets(a, session.ID, args)
			if err != nil {
				return err
			}

			diff, err := a.Checkpoint.Diff(ctx(), beforeID, afterID)
			if err != nil {
				return err
			}

			if !diff.HasChanges() {
				fmt.Println("No changes between checkpoints.")
				return nil
			}

			renderDiff(beforeID, afterID, diff)
			return nil
		},
	}
	return cmd
}

func resolveDiffTargets(a *bootstrap.App, sessionID string, args []string) (string, string, error) {
	checkpoints, err := a.Checkpoint.ListBySession(ctx(), sessionID, 10)
	if err != nil {
		return "", "", err
	}
	if len(checkpoints) < 2 {
		return "", "", fmt.Errorf("need at least two checkpoints to diff (found %d)", len(checkpoints))
	}

	switch len(args) {
	case 0:
		return checkpoints[1].ID, checkpoints[0].ID, nil
	case 1:
		return args[0], checkpoints[0].ID, nil
	default:
		return args[0], args[1], nil
	}
}

func renderDiff(beforeID, afterID string, d *appsvc.SnapshotDiff) {
	if d.Before != nil && d.Before.Label != "" {
		fmt.Printf("Before: %s (%s)\n", shortID(beforeID), d.Before.Label)
	} else {
		fmt.Printf("Before: %s\n", shortID(beforeID))
	}
	if d.After != nil && d.After.Label != "" {
		fmt.Printf("After:  %s (%s)\n\n", shortID(afterID), d.After.Label)
	} else {
		fmt.Printf("After:  %s\n\n", shortID(afterID))
	}

	if d.TaskTitleChanged {
		fmt.Printf("Task title changed\n")
	}
	if d.TaskStatusFrom != d.TaskStatusTo {
		fmt.Printf("Task status: %s → %s\n", d.TaskStatusFrom, d.TaskStatusTo)
	}

	for _, dec := range d.NewDecisions {
		line := "+ Decision: " + dec.Decision
		if dec.Reason != "" {
			line += " — (" + dec.Reason + ")"
		}
		green("%s\n", line)
	}

	for _, b := range d.NewBlockers {
		red("+ Blocker: %s\n", b.Description)
	}
	for _, b := range d.ResolvedBlockers {
		green("✓ Resolved: %s\n", b.Description)
	}

	for _, t := range d.NewlyCompleted {
		green("✓ Completed: %s\n", t)
	}
	for _, t := range d.NewlyStarted {
		fmt.Printf("• Started: %s\n", t)
	}
	// transitions into any other state (blocked, cancelled) are real signals and
	// would otherwise be invisible, since only started/completed print above
	for _, tr := range d.TaskTransitions {
		switch tr.To {
		case entities.TaskStatusCompleted, entities.TaskStatusInProgress:
			continue
		}
		if tr.From == "" {
			yellow("• %s: %s\n", tr.Title, tr.To)
			continue
		}
		yellow("• %s: %s → %s\n", tr.Title, tr.From, tr.To)
	}

	for _, f := range d.NewFiles {
		fmt.Printf("  Changed file: %s\n", f)
	}

	if d.NextActionFrom != d.NextActionTo {
		fmt.Printf("Next action: %s → %s\n", d.NextActionFrom, d.NextActionTo)
	}

	if d.CommitFrom != d.CommitTo {
		fmt.Printf("Commit: %s → %s\n", shortID(d.CommitFrom), shortID(d.CommitTo))
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
