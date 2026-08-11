package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/pkg/version"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "agent-session",
		Short:         "Universal session & handoff layer for AI coding agents",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newInitCmd(),
		newVersionCmd(),
		newStatusCmd(),
		newDashboardCmd(),
		newStatsCmd(),
		newTimelineCmd(),
		newStartCmd(),
		newResumeCmd(),
		newCheckpointCmd(),
		newDiffCmd(),
		newProjectsCmd(),
		newExportCmd(),
		newImportCmd(),
		newHandoffCmd(),
		newHistoryCmd(),
		newContextCmd(),
		newWatchCmd(),
		newDoctorCmd(),
		newMCPCmd(),
		newPluginCmd(),
		newMemoryCmd(),
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

func green(format string, a ...any) {
	fmt.Printf("\033[32m"+format+"\033[0m", a...)
}

func yellow(format string, a ...any) {
	fmt.Printf("\033[33m"+format+"\033[0m", a...)
}

func red(format string, a ...any) {
	fmt.Printf("\033[31m"+format+"\033[0m", a...)
}
