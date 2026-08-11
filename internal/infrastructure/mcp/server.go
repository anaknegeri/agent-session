package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/anaknegeri/agent-session/internal/bootstrap"
	"github.com/anaknegeri/agent-session/pkg/version"
)

const (
	serverName   = "agent-session"
	DefaultAgent = "mcp"
)

var serverVersion = version.Version

type Server struct {
	root           string
	mu             sync.Mutex
	app            *bootstrap.App
	logger         *slog.Logger
	mcp            *server.MCPServer
	lastCheckpoint time.Time
}

// New builds a server rooted at root. The project may not be initialized yet:
// the first tool/resource call resolves it lazily, so a server started before
// `agent-session init` starts working as soon as init completes.
func New(root string, logger *slog.Logger) *Server {
	s := &Server{root: root, logger: logger}
	s.mcp = server.NewMCPServer(serverName, serverVersion, server.WithLogging())
	s.registerTools()
	s.registerResources()
	return s
}

// getApp lazily opens the project app, caching it once successful. Each call
// re-attempts until init exists, so an "always-on" server recovers on its own.
func (s *Server) getApp() (*bootstrap.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app != nil {
		return s.app, nil
	}
	app, err := bootstrap.Open(s.root)
	if err != nil {
		return nil, err
	}
	s.app = app
	return s.app, nil
}

// ready reports whether the project app is usable.
func (s *Server) ready() error {
	_, err := s.getApp()
	return err
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
	app, err := s.getApp()
	if err != nil {
		return "", err
	}
	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		return "", err
	}
	session, err := app.Session.GetActive(ctx, projectID)
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

// maybeCheckpoint auto-creates a checkpoint when auto_checkpoint is enabled.
// In smart mode, rate-limits to at most one checkpoint per 60 seconds and
// skips non-significant events.
func (s *Server) maybeCheckpoint(ctx context.Context, sessionID, reason string) error {
	app, err := s.getApp()
	if err != nil {
		return err
	}
	if app.Cfg == nil || !app.Cfg.Session.AutoCheckpoint {
		return nil
	}
	if app.Cfg.Session.SmartCheckpoint {
		if time.Since(s.lastCheckpoint) < 60*time.Second {
			return nil
		}
	}
	_, err = app.Checkpoint.Create(ctx, sessionID, reason, "", s.agent())
	if err == nil {
		s.lastCheckpoint = time.Now()
	}
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

func intArg(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.RawMessage:
		var f float64
		if json.Unmarshal(t, &f) == nil {
			return int(f)
		}
	}
	return 0
}
