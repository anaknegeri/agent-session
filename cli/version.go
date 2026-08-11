package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/pkg/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show build version, commit and date",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Full())
		},
	}
}
