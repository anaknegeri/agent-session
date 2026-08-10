package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/agent-session/agent-session/internal/bootstrap"
)

const (
	serverName    = "agent-session"
	serverVersion = "0.1.0"
	DefaultAgent  = "mcp"
)

type Server struct {
	app      *bootstrap.App
	notReady error // when set, every tool/resource returns this error
	logger   *slog.Logger
	mcp      *server.MCPServer
}

func New(app *bootstrap.App, logger *slog.Logger) *Server {
	s := &Server{app: app, logger: logger}
	s.mcp = server.NewMCPServer(serverName, serverVersion, server.WithLogging())
	s.registerTools()
	s.registerResources()
	return s
}

// NewNotReady builds a server that stays connected but answers every tool and
// resource call with err. Used for user-scoped servers spawned in projects that
// have not been initialized yet, so the client never sees "connection failed".
func NewNotReady(err error, logger *slog.Logger) *Server {
	s := &Server{notReady: err, logger: logger}
	s.mcp = server.NewMCPServer(serverName, serverVersion, server.WithLogging())
	s.registerTools()
	s.registerResources()
	return s
}

// ready reports whether the app is usable; notReady is prioritized.
func (s *Server) ready() error {
	if s.notReady != nil {
		return s.notReady
	}
	if s.app == nil {
		return fmt.Errorf("agent-session server is not ready")
	}
	return nil
}

// MCPServer exposes the underlying server (for tests / in-process clients).
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcp
}

func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcp)
}

// ServeStreamableHTTP serves over HTTP on the given listener.
// The listener is provided by the caller (see pkg/port for unique ports).
func (s *Server) ServeStreamableHTTP(ln net.Listener) error {
	handler := server.NewStreamableHTTPServer(s.mcp)
	return http.Serve(ln, handler)
}

// currentSession resolves the active session for the project root.
func (s *Server) currentSession(ctx context.Context) (string, error) {
	projectID, err := s.app.ResolveProjectID(ctx, s.app.Root)
	if err != nil {
		return "", err
	}
	session, err := s.app.Session.GetActive(ctx, projectID)
	if err != nil {
		return "", err
	}
	return session.ID, nil
}

// agent returns the caller agent, overridable via AGENT_SESSION_AGENT.
func (s *Server) agent() string {
	if a := os.Getenv("AGENT_SESSION_AGENT"); a != "" {
		return a
	}
	return DefaultAgent
}

// maybeCheckpoint auto-creates a checkpoint when auto_checkpoint is enabled
// (PRD §23 automatic: task completed, test completed, major decision).
func (s *Server) maybeCheckpoint(ctx context.Context, sessionID, reason string) error {
	if s.app.Cfg == nil || !s.app.Cfg.Session.AutoCheckpoint {
		return nil
	}
	_, err := s.app.Checkpoint.Create(ctx, sessionID, reason, "", s.agent())
	return err
}

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.RawMessage:
		var s string
		if json.Unmarshal(t, &s) == nil {
			return s
		}
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
