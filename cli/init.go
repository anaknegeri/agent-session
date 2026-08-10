package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Agent Session in the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := bootstrap.Init(cwd(), "cli")
			if err != nil {
				return err
			}
			fmt.Printf("Project:   %s\n", app.Root)
			fmt.Printf("Session:   %s\n", sessionID(app))
			fmt.Printf("Status:    active\n")
			fmt.Printf("Data:      %s/.agent\n", app.Root)
			return nil
		},
	}
}

func sessionID(app *bootstrap.App) string {
	projectID, err := app.ResolveProjectID(ctx(), app.Root)
	if err != nil {
		return ""
	}
	session, err := app.Session.GetActive(ctx(), projectID)
	if err != nil {
		return ""
	}
	return session.ID
}
