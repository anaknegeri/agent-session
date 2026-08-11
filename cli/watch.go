package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	var interval int
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch and auto-regenerate context.md when the session changes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}

			d := time.Duration(interval) * time.Second
			ticker := time.NewTicker(d)
			defer ticker.Stop()

			fmt.Printf("Watching session %s (refresh every %ds, Ctrl+C to stop)...\n\n", session.ID, interval)

			lastEventCount := 0
			for range ticker.C {
				events, err := app.Event.List(ctx(), session.ID, 500)
				if err != nil {
					continue
				}
				if len(events) == lastEventCount {
					continue
				}
				lastEventCount = len(events)

				if _, err := app.Context.WriteContextMD(ctx(), app.Root, session.ID); err != nil {
					red("✗ %v\n", err)
					continue
				}
				if len(events) > 0 {
					e := events[0]
					green("✓ %s  %s [%s]  → .agent/context/current.md\n", e.CreatedAt.Format("15:04:05"), e.Type, e.Agent)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&interval, "interval", "i", 5, "Polling interval in seconds")
	return cmd
}
