package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/agent-session/agent-session/internal/bootstrap"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "agent-session",
		Short:         "Universal session & handoff layer for AI coding agents",
		Version:       "0.1.0",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newInitCmd(),
		newStatusCmd(),
		newStartCmd(),
		newResumeCmd(),
		newCheckpointCmd(),
		newHandoffCmd(),
		newHistoryCmd(),
		newContextCmd(),
		newDoctorCmd(),
		newMCPCmd(),
		newPluginCmd(),
		newSetupCmd(),
	)
	return root
}

// open resolves the project app from cwd or exits with a helpful error.
func open() *bootstrap.App {
	app, err := bootstrap.Open(".")
	if err != nil {
		fail(err)
	}
	return app
}

func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fail(fmt.Errorf("get cwd: %w", err))
	}
	return dir
}

func ctx() context.Context {
	return context.Background()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Error: "+err.Error())
	os.Exit(1)
}
