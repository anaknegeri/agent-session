package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var format string
	var output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the current session to JSON or Markdown",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			session, err := currentSession(app)
			if err != nil {
				return err
			}

			if format == "json" {
				data, err := app.Export.ExportJSON(ctx(), session.ID)
				if err != nil {
					return err
				}
				return writeOutput(output, data)
			}

			text, err := app.Export.ExportMarkdown(ctx(), session.ID)
			if err != nil {
				return err
			}
			return writeOutput(output, []byte(text))
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json | markdown")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (defaults to stdout)")
	return cmd
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import a session from a JSON export file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := open()
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}
			projectID, err := app.ResolveProjectID(ctx(), app.Root)
			if err != nil {
				return err
			}
			sessionID, err := app.Export.Import(ctx(), projectID, data, "cli")
			if err != nil {
				return err
			}
			fmt.Printf("Session imported: %s\n", sessionID)
			return nil
		},
	}
	return cmd
}

func writeOutput(path string, data []byte) error {
	if path == "" || path == "-" {
		os.Stdout.Write(data)
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}
