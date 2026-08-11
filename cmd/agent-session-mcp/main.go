package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/internal/infrastructure/mcp"
	"github.com/anaknegeri/agent-session/pkg/logger"
	"github.com/anaknegeri/agent-session/pkg/port"
)

func main() {
	transport := flag.String("transport", "stdio", "stdio | streamable-http")
	addr := flag.String("addr", "auto", "HTTP listen address (streamable-http). auto picks a unique port per project")
	flag.Parse()

	root, err := bootstrap.ResolveRoot(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	server := mcp.New(root, logger.New("info"))
	switch *transport {
	case "stdio":
		if err := server.ServeStdio(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "streamable-http":
		if *addr == "auto" {
			ln, p, err := port.Listen(root, port.DefaultBase, port.DefaultSpan)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			defer ln.Close()
			fmt.Fprintf(os.Stderr, "agent-session MCP listening on http://127.0.0.1:%d/mcp\n", p)
			if err := server.ServeStreamableHTTP(ln); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			return
		}
		if err := server.ServeStreamableHTTP(listenAddr(*addr)); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown transport", *transport)
		os.Exit(1)
	}
}

// listenAddr binds addr ("host:port") and returns the listener.
func listenAddr(addr string) net.Listener {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	return ln
}
