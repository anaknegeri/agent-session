package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agent-session/agent-session/internal/bootstrap"
	"github.com/agent-session/agent-session/internal/domain/entities"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current session status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}

			snapshot, err := app.Checkpoint.BuildSnapshot(ctx(), session.ID)
			if err != nil {
				return err
			}

			fmt.Printf("Project       %s\n", snapshot.Workspace.Repository)
			fmt.Printf("Session       %s\n", snapshot.Session.Title)
			fmt.Printf("Status        %s\n", title(snapshot.Session.Status))
			fmt.Printf("Last Agent    %s\n", snapshot.LastAgent)
			fmt.Printf("Branch        %s\n", snapshot.Workspace.Branch)
			fmt.Printf("\n")

			total := len(snapshot.Progress.Completed) + len(snapshot.Progress.Pending)
			if total > 0 {
				pct := len(snapshot.Progress.Completed) * 100 / total
				fmt.Printf("Progress      %d%%\n", pct)
			}

			if len(snapshot.Progress.Completed) > 0 {
				fmt.Printf("\nCompleted\n")
				for _, item := range snapshot.Progress.Completed {
					fmt.Printf("  ✓ %s\n", item)
				}
			}
			if len(snapshot.Progress.Pending) > 0 {
				fmt.Printf("\nIn progress\n")
				for _, item := range snapshot.Progress.Pending {
					fmt.Printf("  • %s\n", item)
				}
			}
			if len(snapshot.Blockers) > 0 {
				fmt.Printf("\nBlocked\n")
				for _, b := range snapshot.Blockers {
					fmt.Printf("  ✗ %s\n", b.Description)
				}
			}
			if snapshot.NextAction != "" {
				fmt.Printf("\nNext\n  %s\n", snapshot.NextAction)
			}
			return nil
		},
	}
}

func currentSession(app *bootstrap.App) (*entities.Session, error) {
	projectID, err := app.ResolveProjectID(ctx(), app.Root)
	if err != nil {
		return nil, err
	}
	return app.Session.GetActive(ctx(), projectID)
}

func title(s string) string {
	switch s {
	case entities.SessionStatusActive:
		return "Active"
	case entities.SessionStatusPaused:
		return "Paused"
	case entities.SessionStatusCompleted:
		return "Completed"
	}
	return s
}
