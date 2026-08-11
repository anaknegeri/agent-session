package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func newDashboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Show a comprehensive dashboard of the current session",
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

			cps, _ := app.Checkpoint.ListBySession(ctx(), session.ID, 5)
			events, _ := app.Event.List(ctx(), session.ID, 15)

			renderDashboard(snapshot, cps, events)
			return nil
		},
	}
}

func renderDashboard(s *entities.Snapshot, cps []*entities.Checkpoint, events []*entities.SessionEvent) {
	header()
	fmt.Printf("  Session:   %s\n", s.Session.Title)
	fmt.Printf("  Status:    %s\n", s.Session.Status)
	fmt.Printf("  Project:   %s\n", s.Workspace.Repository)
	fmt.Printf("  Branch:    %s\n", s.Workspace.Branch)
	if s.Workspace.Commit != "" {
		commit := s.Workspace.Commit
		if len(commit) > 12 {
			commit = commit[:12]
		}
		dirty := ""
		if s.Workspace.Dirty {
			dirty = " (dirty)"
		}
		fmt.Printf("  Commit:    %s%s\n", commit, dirty)
	}
	fmt.Printf("  Agent:     %s\n", s.LastAgent)
	fmt.Println()

	if s.Task.Title != "" {
		fmt.Printf("  Task:      %s (%s)\n\n", s.Task.Title, s.Task.Status)
	}

	total := len(s.Progress.Completed) + len(s.Progress.Pending)
	if total > 0 {
		pct := 0
		if total > 0 {
			pct = len(s.Progress.Completed) * 100 / total
		}
		bar := progressBar(pct, 30)
		fmt.Printf("  Progress:  %s %d%% (%d/%d)\n\n", bar, pct, len(s.Progress.Completed), total)
	}

	if len(s.Progress.Completed) > 0 {
		green("  Completed:\n")
		for _, item := range s.Progress.Completed {
			green("    ✓ %s\n", item)
		}
		fmt.Println()
	}
	if len(s.Progress.Pending) > 0 {
		yellow("  In progress:\n")
		for _, item := range s.Progress.Pending {
			fmt.Printf("    • %s\n", item)
		}
		fmt.Println()
	}

	if len(s.Blockers) > 0 {
		red("  Blockers:\n")
		for _, b := range s.Blockers {
			red("    ✗ %s\n", b.Description)
		}
		fmt.Println()
	}

	if len(s.Decisions) > 0 {
		fmt.Printf("  Decisions (%d):\n", len(s.Decisions))
		limit := len(s.Decisions)
		if limit > 5 {
			limit = 5
		}
		for _, d := range s.Decisions[:limit] {
			line := truncate(d.Decision, 70)
			fmt.Printf("    → %s\n", line)
		}
		if len(s.Decisions) > 5 {
			fmt.Printf("    … +%d more\n", len(s.Decisions)-5)
		}
		fmt.Println()
	}

	if s.Tests.Status != "" && s.Tests.Status != "unknown" {
		icon := "✓"
		color := green
		if s.Tests.Status == "failed" {
			icon = "✗"
			color = red
		}
		color("  Tests:     %s %s", icon, s.Tests.Status)
		if s.Tests.Failures > 0 {
			color(" (%d failures)", s.Tests.Failures)
		}
		color("\n\n")
	}

	if len(cps) > 0 {
		fmt.Printf("  Recent checkpoints:\n")
		for _, cp := range cps {
			label := cp.Label
			if label == "" {
				label = truncate(cp.NextAction, 50)
			}
			if label == "" {
				label = "—"
			}
			fmt.Printf("    %s  %s  [%s]\n", cp.CreatedAt.Format("Jan 02 15:04"), shortID(cp.ID), label)
		}
		fmt.Println()
	}

	if len(events) > 0 {
		fmt.Printf("  Recent events:\n")
		for _, e := range events {
			fmt.Printf("    %s  %-22s  [%s]\n", e.CreatedAt.Format("Jan 02 15:04"), e.Type, e.Agent)
		}
		fmt.Println()
	}

	if s.NextAction != "" {
		green("  Next: %s\n", s.NextAction)
	}
}

func header() {
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════════════╗")
	fmt.Println("  ║              AGENT SESSION — DASHBOARD                   ║")
	fmt.Println("  ╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}
