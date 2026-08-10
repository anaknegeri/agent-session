package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "start [title]",
		Short: "Start a new session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && title == "" {
				title = args[0]
			}
			app := open()
			projectID, err := app.ResolveProjectID(ctx(), app.Root)
			if err != nil {
				return err
			}
			session, err := app.Session.Start(ctx(), projectID, title, "cli")
			if err != nil {
				return err
			}
			fmt.Printf("Session started: %s\n", session.ID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Session title")
	return cmd
}

func newResumeCmd() *cobra.Command {
	var agent string
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume the latest session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			projectID, err := app.ResolveProjectID(ctx(), app.Root)
			if err != nil {
				return err
			}
			session, err := app.Session.Resume(ctx(), projectID, agent)
			if err != nil {
				return err
			}
			text, err := app.Context.Get(ctx(), session.ID, "summary")
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
	cmd.Flags().StringVarP(&agent, "agent", "a", "cli", "Agent name")
	return cmd
}

func newCheckpointCmd() *cobra.Command {
	var label, nextAction string
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Create a checkpoint snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			cp, err := app.Checkpoint.Create(ctx(), session.ID, label, nextAction, "cli")
			if err != nil {
				return err
			}
			if _, err := app.Context.WriteContextMD(ctx(), app.Root, session.ID); err != nil {
				return err
			}
			fmt.Printf("Checkpoint created: %s\n", cp.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "Checkpoint label")
	cmd.Flags().StringVarP(&nextAction, "next-action", "n", "", "Next action")
	return cmd
}

func newHandoffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "handoff <agent>",
		Short: "Create handoff context for another agent (claude, codex, opencode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			text, err := app.Handoff.Handoff(ctx(), session.ID, args[0])
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
}

func newHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show recent session events",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			events, err := app.Event.List(ctx(), session.ID, 50)
			if err != nil {
				return err
			}
			for _, e := range events {
				fmt.Printf("%s  %-22s %s\n", e.CreatedAt.Format("2006-01-02 15:04:05"), e.Type, e.Agent)
			}
			return nil
		},
	}
}

func newContextCmd() *cobra.Command {
	var depth string
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print the current session context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			if depth == "" {
				depth = "summary"
			}
			text, err := app.Context.Get(ctx(), session.ID, depth)
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
	cmd.Flags().StringVarP(&depth, "depth", "d", "summary", "summary|recent|full")
	return cmd
}
