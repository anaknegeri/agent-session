package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// registerResources exposes read-only MCP resources (PRD §19).
func (s *Server) registerResources() {
	resource := func(uri, name, desc string, handler server.ResourceHandlerFunc) {
		s.mcp.AddResource(
			mcp.NewResource(uri, name, mcp.WithResourceDescription(desc)),
			handler,
		)
	}

	resource("session://current", "Current session",
		"Active session state",
		sessionResource(s, func(ctx context.Context, sessionID string) (any, error) {
			return s.app.Session.Get(ctx, sessionID)
		}))

	resource("session://context", "Current context",
		"Rendered context.md",
		sessionResource(s, func(ctx context.Context, sessionID string) (any, error) {
			// Read, not Get: resources are read-only by contract, and Get syncs file
			// changes and may auto-checkpoint. A client polling this resource was
			// writing to the session on every poll while believing it only read.
			return s.app.Context.Read(ctx, sessionID, "summary")
		}))

	resource("session://decisions", "Decisions",
		"List of recorded decisions",
		sessionResource(s, func(ctx context.Context, sessionID string) (any, error) {
			return s.app.Decision.List(ctx, sessionID)
		}))

	resource("session://tasks", "Tasks",
		"List of tasks",
		sessionResource(s, func(ctx context.Context, sessionID string) (any, error) {
			return s.app.Task.List(ctx, sessionID)
		}))

	resource("session://workspace", "Workspace",
		"Git workspace status",
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			app, err := s.getApp()
			if err != nil {
				return nil, err
			}
			data, err := app.Workspace.Status(ctx, app.Root)
			if err != nil {
				return nil, err
			}
			return resourceText(req.Params.URI, data)
		})

	resource("session://checkpoint/latest", "Latest checkpoint",
		"Most recent checkpoint snapshot",
		sessionResource(s, func(ctx context.Context, sessionID string) (any, error) {
			return s.app.Checkpoint.Latest(ctx, sessionID)
		}))

	resource("memory://recent", "Recent knowledge",
		"Recent long-term memory entries",
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			app, err := s.getApp()
			if err != nil {
				return nil, err
			}
			rows, err := app.Memory.ListByKind(ctx, "", 10)
			if err != nil {
				return nil, err
			}
			return resourceText(req.Params.URI, rows)
		})
}

// sessionResource resolves the current session and delegates to fn.
func sessionResource(s *Server, fn func(ctx context.Context, sessionID string) (any, error)) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if err := s.ready(); err != nil {
			return nil, err
		}
		sessionID, err := s.currentSession(ctx)
		if err != nil {
			return nil, err
		}
		data, err := fn(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return resourceText(req.Params.URI, data)
	}
}

// resourceText wraps data as the contents of the requested URI. The URI has to
// be echoed back: a client matches contents against the URI it asked for, and a
// single hardcoded one labelled every resource as the same thing.
func resourceText(uri string, data any) ([]mcp.ResourceContents, error) {
	switch v := data.(type) {
	case string:
		return []mcp.ResourceContents{
			mcp.TextResourceContents{URI: uri, Text: v},
		}, nil
	case *entities.SessionEvent:
		return []mcp.ResourceContents{
			mcp.TextResourceContents{URI: uri, Text: v.Type},
		}, nil
	default:
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal resource: %w", err)
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{URI: uri, Text: string(raw)},
		}, nil
	}
}
