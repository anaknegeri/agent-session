package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// The commands in this file mirror the MCP write tools (task.*, decision.*,
// blocker.*, event.append) for agents that have no MCP client at all — pi being
// the first one. Without them, such an agent can read a session but never
// contribute to it, which makes the session layer write-only from one side.

// recordingAgent names the writer of a record. Agents set AGENT_SESSION_AGENT in
// their own environment so their writes are attributed to them rather than to
// the generic CLI.
func recordingAgent(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("AGENT_SESSION_AGENT"); env != "" {
		return env
	}
	return "cli"
}

func addAgentFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVarP(target, "agent", "a", "",
		"Agent recording this (default $AGENT_SESSION_AGENT or \"cli\")")
}

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Record and inspect tasks",
	}
	cmd.AddCommand(newTaskAddCmd(), newTaskListCmd(), newTaskUpdateCmd())
	return cmd
}

func newTaskAddCmd() *cobra.Command {
	var agent string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a task and make it current",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			task, err := app.Task.Create(ctx(), session.ID, args[0], recordingAgent(agent))
			if err != nil {
				return err
			}
			fmt.Printf("task created: %s\n", task.ID)
			return nil
		},
	}
	addAgentFlag(cmd, &agent)
	return cmd
}

func newTaskListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tasks of the current session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			tasks, err := app.Task.List(ctx(), session.ID)
			if err != nil {
				return err
			}
			for _, t := range tasks {
				fmt.Printf("%s  %-12s %s\n", t.ID, t.Status, singleLine(t.Title))
			}
			return nil
		},
	}
}

func newTaskUpdateCmd() *cobra.Command {
	var agent, title, status string
	cmd := &cobra.Command{
		Use:   "update <task-id>",
		Short: "Update a task title and/or status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" && status == "" {
				return fmt.Errorf("nothing to update: pass --title and/or --status")
			}
			app := open()
			task, err := app.Task.Update(ctx(), args[0], title, status, recordingAgent(agent))
			if err != nil {
				return err
			}
			fmt.Printf("task updated: %s (%s)\n", task.ID, task.Status)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "New title")
	cmd.Flags().StringVarP(&status, "status", "s", "",
		"in_progress|completed|blocked|cancelled")
	addAgentFlag(cmd, &agent)
	return cmd
}

func newDecisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Record and inspect decisions",
	}
	cmd.AddCommand(newDecisionAddCmd(), newDecisionListCmd())
	return cmd
}

func newDecisionAddCmd() *cobra.Command {
	var agent, reason string
	cmd := &cobra.Command{
		Use:   "add <decision>",
		Short: "Record a decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			d, err := app.Decision.Create(ctx(), session.ID, args[0], reason, recordingAgent(agent))
			if err != nil {
				return err
			}
			fmt.Printf("decision recorded: %s\n", d.ID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "Why this was decided")
	addAgentFlag(cmd, &agent)
	return cmd
}

func newDecisionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List decisions of the current session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			rows, err := app.Decision.List(ctx(), session.ID)
			if err != nil {
				return err
			}
			for _, d := range rows {
				fmt.Printf("%s  %s\n", d.ID, singleLine(d.Decision))
				if d.Reason != "" {
					fmt.Printf("            why: %s\n", singleLine(d.Reason))
				}
			}
			return nil
		},
	}
}

func newBlockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocker",
		Short: "Record and resolve blockers",
	}
	cmd.AddCommand(newBlockerAddCmd(), newBlockerListCmd(), newBlockerResolveCmd())
	return cmd
}

func newBlockerAddCmd() *cobra.Command {
	var agent string
	cmd := &cobra.Command{
		Use:   "add <description>",
		Short: "Record a blocker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			b, err := app.Decision.CreateBlocker(ctx(), session.ID, args[0], recordingAgent(agent))
			if err != nil {
				return err
			}
			fmt.Printf("blocker recorded: %s\n", b.ID)
			return nil
		},
	}
	addAgentFlag(cmd, &agent)
	return cmd
}

func newBlockerListCmd() *cobra.Command {
	var openOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List blockers of the current session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			rows, err := app.Decision.ListBlockers(ctx(), session.ID, openOnly)
			if err != nil {
				return err
			}
			for _, b := range rows {
				fmt.Printf("%s  %-9s %s\n", b.ID, b.Status, singleLine(b.Description))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&openOnly, "open", false, "Only unresolved blockers")
	return cmd
}

func newBlockerResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <blocker-id>",
		Short: "Mark a blocker resolved",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			if err := app.Decision.ResolveBlocker(ctx(), args[0]); err != nil {
				return err
			}
			fmt.Printf("blocker resolved: %s\n", args[0])
			return nil
		},
	}
}

func newEventCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Append canonical session events",
	}
	cmd.AddCommand(newEventAddCmd())
	return cmd
}

func newEventAddCmd() *cobra.Command {
	var agent, payload string
	cmd := &cobra.Command{
		Use:   "add <type>",
		Short: "Append a canonical event (see docs/spec/event-v1.md)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Names the accepted values before touching the store: the type
			// namespace is closed, and "unknown canonical event type" alone does
			// not tell a caller what to send instead.
			if !entities.IsCanonicalEventType(args[0]) {
				return fmt.Errorf("unknown event type %q, expected one of: %s",
					args[0], strings.Join(entities.CanonicalEventTypes(), ", "))
			}
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			// Artifact.AppendEvent, not Event.Append: it is the path MCP uses, so
			// oversized payloads are offloaded to an artifact here too.
			if err := app.Artifact.AppendEvent(ctx(), session.ID, recordingAgent(agent), args[0], payload); err != nil {
				return err
			}
			fmt.Printf("event appended: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&payload, "payload", "p", "", "Payload (JSON or free text)")
	addAgentFlag(cmd, &agent)
	return cmd
}
