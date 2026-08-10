package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Long-term knowledge store (Phase 4)",
	}
	cmd.AddCommand(
		newMemoryPutCmd(),
		newMemoryListCmd(),
		newMemorySearchCmd(),
		newMemoryPromoteCmd(),
	)
	return cmd
}

func newMemoryPutCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "put <content>",
		Short: "Store a piece of knowledge",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			sessionID := ""
			if err == nil {
				sessionID = session.ID
			}
			k, err := app.Memory.Put(ctx(), sessionID, kind, args[0], "cli")
			if err != nil {
				return err
			}
			fmt.Printf("stored: %s (%s)\n", k.ID, kind)
			return nil
		},
	}
	cmd.Flags().StringVarP(&kind, "kind", "k", entities.KnowledgeKindProject,
		"project_knowledge|architecture|solution|preference|skill")
	return cmd
}

func newMemoryListCmd() *cobra.Command {
	var kind string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List knowledge",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			rows, err := app.Memory.ListByKind(ctx(), kind, limit)
			if err != nil {
				return err
			}
			for _, k := range rows {
				fmt.Printf("%s  [%s]  %s\n", k.ID, k.Kind, k.Content)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&kind, "kind", "k", "", "filter by kind")
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "max results")
	return cmd
}

func newMemorySearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search the knowledge store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			hits, err := app.Memory.Search(ctx(), args[0], limit)
			if err != nil {
				return err
			}
			for _, h := range hits {
				fmt.Printf("%s  [%s]  %s\n", h.ID, h.Kind, h.Content)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "max results")
	return cmd
}

func newMemoryPromoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote",
		Short: "Promote session decisions/blockers/tasks into memory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}
			count, err := app.Memory.Promote(ctx(), session.ID, "cli")
			if err != nil {
				return err
			}
			fmt.Printf("promoted %d item(s)\n", count)
			return nil
		},
	}
}
