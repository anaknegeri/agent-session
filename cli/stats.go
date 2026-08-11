package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show session statistics and insights",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}

			tasks, _ := app.Task.List(ctx(), session.ID)
			decisions, _ := app.Decision.List(ctx(), session.ID)
			blockers, _ := app.Decision.ListBlockers(ctx(), session.ID, false)
			events, _ := app.Event.List(ctx(), session.ID, 500)
			cps, _ := app.Checkpoint.ListBySession(ctx(), session.ID, 500)

			completed := 0
			cancelled := 0
			inProgress := 0
			blocked := 0
			for _, t := range tasks {
				switch t.Status {
				case "completed":
					completed++
				case "cancelled":
					cancelled++
				case "in_progress":
					inProgress++
				case "blocked":
					blocked++
				}
			}

			openBlockers := 0
			resolvedBlockers := 0
			for _, b := range blockers {
				if b.Status == "open" {
					openBlockers++
				} else {
					resolvedBlockers++
				}
			}

			testPassed := 0
			testFailed := 0
			for _, e := range events {
				switch e.Type {
				case "test.passed":
					testPassed++
				case "test.failed":
					testFailed++
				}
			}

			uniqueAgents := map[string]bool{}
			for _, e := range events {
				uniqueAgents[e.Agent] = true
			}

			fmt.Println()
			fmt.Println("  ╔══════════════════════════════════════════════════════════╗")
			fmt.Println("  ║               AGENT SESSION — STATS                      ║")
			fmt.Println("  ╚══════════════════════════════════════════════════════════╝")
			fmt.Println()
			fmt.Printf("  Session:     %s\n\n", truncate(session.Title, 50))

			fmt.Printf("  Tasks\n")
			fmt.Printf("    Total:       %d\n", len(tasks))
			fmt.Printf("    Completed:   %d\n", completed)
			fmt.Printf("    In progress: %d\n", inProgress)
			fmt.Printf("    Blocked:     %d\n", blocked)
			fmt.Printf("    Cancelled:   %d\n\n", cancelled)

			fmt.Printf("  Decisions:    %d\n", len(decisions))
			fmt.Printf("  Blockers:     %d open, %d resolved\n\n", openBlockers, resolvedBlockers)

			fmt.Printf("  Checkpoints:  %d\n", len(cps))
			fmt.Printf("  Events:       %d\n", len(events))
			fmt.Printf("  Tests:        %d passed, %d failed\n\n", testPassed, testFailed)

			fmt.Printf("  Agents used:  %d (", len(uniqueAgents))
			first := true
			for a := range uniqueAgents {
				if !first {
					fmt.Printf(", ")
				}
				fmt.Printf("%s", a)
				first = false
			}
			fmt.Printf(")\n")

			if len(tasks) > 0 {
				pct := completed * 100 / len(tasks)
				fmt.Printf("\n  Completion:   %s %d%%\n", progressBar(pct, 25), pct)
			}

			if !session.CreatedAt.IsZero() {
				fmt.Printf("\n  Created:      %s\n", session.CreatedAt.Format("2006-01-02 15:04"))
			}
			fmt.Println()
			return nil
		},
	}
}
