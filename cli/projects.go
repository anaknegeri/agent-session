package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

func newProjectsCmd() *cobra.Command {
	var prune bool
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List all projects with Agent Session initialized",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := loadRegistry()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No registered projects. Run `agent-session init` in a project to register it.")
				return nil
			}

			if prune {
				return pruneStale(entries)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tPATH\tSESSION\tSTATUS\tLAST AGENT")

			for _, e := range entries {
				if _, err := os.Stat(e.Path); os.IsNotExist(err) {
					fmt.Fprintf(w, "%s\t%s\t—\t(missing)\t—\n", e.Name, e.Path)
					continue
				}
				renderProjectRow(w, e)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "Remove projects whose directories no longer exist")
	return cmd
}

func renderProjectRow(w *tabwriter.Writer, e registryEntry) {
	projectName := e.Name
	sessionTitle := "—"
	status := "—"
	lastAgent := "—"

	if app, err := bootstrap.Open(e.Path); err == nil {
		if projectID, err := app.ResolveProjectID(ctx(), app.Root); err == nil {
			if session, err := app.Session.GetLatest(ctx(), projectID); err == nil {
				sessionTitle = truncate(session.Title, 40)
				status = session.Status
				lastAgent = session.LastAgent
			}
		}
		app.Store.Close()
	}

	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", projectName, e.Path, sessionTitle, status, lastAgent)
}

func pruneStale(entries []registryEntry) error {
	var removed int
	for _, e := range entries {
		if _, err := os.Stat(e.Path); os.IsNotExist(err) {
			if err := unregisterProject(e.Path); err != nil {
				return err
			}
			fmt.Printf("Removed: %s (%s)\n", e.Name, e.Path)
			removed++
		}
	}
	if removed == 0 {
		fmt.Println("No stale projects found.")
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
