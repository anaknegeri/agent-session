package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func newTimelineCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Show session events as a visual timeline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}

			events, err := app.Event.List(ctx(), session.ID, limit)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				fmt.Println("No events found.")
				return nil
			}

			fmt.Println()
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				isLast := i == 0
				connector := "├──"
				if isLast {
					connector = "└──"
				}
				icon := eventIcon(e.Type)
				fmt.Printf("  %s %s  %s %-22s  [%s]\n",
					connector, e.CreatedAt.Format("Jan 02 15:04"), icon, e.Type, e.Agent)
			}
			fmt.Println()
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 30, "Maximum number of events")
	return cmd
}

func eventIcon(eventType string) string {
	switch eventType {
	case entities.EventTestPassed:
		return "✓"
	case entities.EventTestFailed:
		return "✗"
	case entities.EventCheckpointCreated:
		return "◆"
	case entities.EventDecisionCreated:
		return "→"
	case entities.EventBlockerCreated:
		return "⚠"
	case entities.EventFileChanged:
		return "✎"
	case entities.EventSessionStarted:
		return "▶"
	case entities.EventTaskUpdated:
		return "✓"
	default:
		return "•"
	}
}
