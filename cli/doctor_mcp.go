package cli

import (
	"fmt"
	"net"

	"github.com/spf13/cobra"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/mcp"
	"github.com/anaknegeri/agent-session/pkg/logger"
	"github.com/anaknegeri/agent-session/pkg/port"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the Agent Session installation and project state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok := true
			check := func(name string, err error) {
				if err != nil {
					ok = false
					fmt.Printf("✗ %s: %v\n", name, err)
					return
				}
				fmt.Printf("✓ %s\n", name)
			}

			app, err := bootstrap.Open(".")
			check("project initialized", err)
			if err != nil {
				return nil
			}

			session, err := currentSession(app)
			check("active session", err)
			if err != nil {
				return nil
			}
			fmt.Printf("  session: %s (%s)\n", session.ID, session.Status)

			check("sqlite store", app.Check())
			check("git workspace", app.WorkspaceCheck())

			if !ok {
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
}

func newMCPCmd() *cobra.Command {
	var transport, addr string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server (stdio or streamable-http)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := bootstrap.Open(".")
			var server *mcp.Server
			if err != nil {
				server = mcp.NewNotReady(err, logger.New("info"))
			} else {
				server = mcp.New(app, logger.New("info"))
			}
			switch transport {
			case "stdio", "":
				return server.ServeStdio()
			case "streamable-http":
				if addr == "auto" {
					root := "."
					if app != nil {
						root = app.Root
					}
					ln, p, err := port.Listen(root, port.DefaultBase, port.DefaultSpan)
					if err != nil {
						return err
					}
					defer ln.Close()
					fmt.Fprintf(cmd.ErrOrStderr(), "agent-session MCP listening on http://127.0.0.1:%d/mcp\n", p)
					return server.ServeStreamableHTTP(ln)
				}
				ln, err := net.Listen("tcp", addr)
				if err != nil {
					return err
				}
				defer ln.Close()
				return server.ServeStreamableHTTP(ln)
			default:
				return fmt.Errorf("unknown transport %q", transport)
			}
		},
	}
	cmd.Flags().StringVar(&transport, "transport", "stdio", "stdio | streamable-http")
	cmd.Flags().StringVar(&addr, "addr", "auto", "HTTP listen address (streamable-http). auto picks a unique port per project")
	return cmd
}
