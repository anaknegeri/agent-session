package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

type toolSpec struct {
	name    string
	desc    string
	options []mcp.ToolOption
	run     func(ctx context.Context, args map[string]any) (any, error)
}

func (s *Server) registerTools() {
	specs := []toolSpec{
		{
			name: "session.get",
			desc: "Get the current session state.",
			options: []mcp.ToolOption{
				mcp.WithString("session_id", mcp.Description("Session ID (defaults to current)")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID := argString(args, "session_id")
				if sessionID == "" {
					return s.currentSession(ctx)
				}
				return s.app.Session.Get(ctx, sessionID)
			},
		},
		{
			name: "session.checkpoint",
			desc: "Create a checkpoint snapshot of the current work.",
			options: []mcp.ToolOption{
				mcp.WithString("label", mcp.Description("Optional checkpoint label")),
				mcp.WithString("next_action", mcp.Description("Optional next action")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				cp, err := s.app.Checkpoint.Create(ctx, sessionID, argString(args, "label"), argString(args, "next_action"), s.agent())
				if err != nil {
					return nil, err
				}
				_, err = s.app.Context.WriteContextMD(ctx, s.app.Root, sessionID)
				return cp, err
			},
		},
		{
			name: "session.resume",
			desc: "Resume the latest session and mark it active.",
			options: []mcp.ToolOption{
				mcp.WithString("agent", mcp.Description("Agent name attaching to the session")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				projectID, err := s.app.ResolveProjectID(ctx, s.app.Root)
				if err != nil {
					return nil, err
				}
				agent := argString(args, "agent")
				if agent == "" {
					agent = s.agent()
				}
				session, err := s.app.Session.Resume(ctx, projectID, agent)
				if err != nil {
					return nil, err
				}
				contextText, cerr := s.app.Context.Get(ctx, session.ID, "summary")
				if cerr != nil {
					return nil, cerr
				}
				return map[string]any{"session": session, "context": contextText}, nil
			},
		},
		{
			name: "context.get",
			desc: "Get the current session context (summary, recent or full).",
			options: []mcp.ToolOption{
				mcp.WithString("depth", mcp.Description("summary|recent|full")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				depth := argString(args, "depth")
				if depth == "" {
					depth = "summary"
				}
				return s.app.Context.Get(ctx, sessionID, depth)
			},
		},
		{
			name: "context.update",
			desc: "Update a context field (task_title, task_status, next_action, session_title).",
			options: []mcp.ToolOption{
				mcp.WithString("field", mcp.Description("task_title|task_status|next_action|session_title"), mcp.Required()),
				mcp.WithString("value", mcp.Description("New value"), mcp.Required()),
			},
			run: s.runContextUpdate,
		},
		{
			name: "task.create",
			desc: "Create a new task and set it as the current task.",
			options: []mcp.ToolOption{
				mcp.WithString("title", mcp.Description("Task title"), mcp.Required()),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				task, err := s.app.Task.Create(ctx, sessionID, argString(args, "title"), s.agent())
				if err != nil {
					return nil, err
				}
				return task, s.maybeCheckpoint(ctx, sessionID, "task created: "+task.Title)
			},
		},
		{
			name: "task.get",
			desc: "Get a task (defaults to the current task).",
			options: []mcp.ToolOption{
				mcp.WithString("task_id", mcp.Description("Task ID")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				taskID := argString(args, "task_id")
				if taskID != "" {
					return s.app.Task.Get(ctx, taskID)
				}
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				return s.app.Task.GetCurrent(ctx, sessionID)
			},
		},
		{
			name: "task.update",
			desc: "Update a task (title, status).",
			options: []mcp.ToolOption{
				mcp.WithString("task_id", mcp.Description("Task ID"), mcp.Required()),
				mcp.WithString("title", mcp.Description("New title")),
				mcp.WithString("status", mcp.Description("in_progress|completed|blocked|cancelled")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				task, err := s.app.Task.Update(ctx,
					argString(args, "task_id"),
					argString(args, "title"),
					argString(args, "status"),
					s.agent())
				if err != nil {
					return nil, err
				}
				return task, s.maybeCheckpoint(ctx, task.SessionID, "task updated: "+task.Title)
			},
		},
		{
			name: "decision.list",
			desc: "List decisions of the current session.",
			options: []mcp.ToolOption{
				mcp.WithString("session_id", mcp.Description("Session ID")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID := argString(args, "session_id")
				if sessionID == "" {
					var err error
					sessionID, err = s.currentSession(ctx)
					if err != nil {
						return nil, err
					}
				}
				return s.app.Decision.List(ctx, sessionID)
			},
		},
		{
			name: "decision.create",
			desc: "Record a decision with an optional reason.",
			options: []mcp.ToolOption{
				mcp.WithString("decision", mcp.Description("The decision"), mcp.Required()),
				mcp.WithString("reason", mcp.Description("Reasoning behind the decision")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				d, err := s.app.Decision.Create(ctx, sessionID, argString(args, "decision"), argString(args, "reason"), s.agent())
				if err != nil {
					return nil, err
				}
				return d, s.maybeCheckpoint(ctx, sessionID, "decision: "+d.Decision)
			},
		},
		{
			name: "blocker.create",
			desc: "Record a blocker.",
			options: []mcp.ToolOption{
				mcp.WithString("description", mcp.Description("Blocker description"), mcp.Required()),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				return s.app.Decision.CreateBlocker(ctx, sessionID, argString(args, "description"), s.agent())
			},
		},
		{
			name: "blocker.list",
			desc: "List blockers of the current session.",
			options: []mcp.ToolOption{
				mcp.WithString("open_only", mcp.Description("Only open blockers")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				openOnly := argString(args, "open_only") == "true"
				return s.app.Decision.ListBlockers(ctx, sessionID, openOnly)
			},
		},
		{
			name: "blocker.resolve",
			desc: "Resolve a blocker by ID.",
			options: []mcp.ToolOption{
				mcp.WithString("blocker_id", mcp.Description("Blocker ID"), mcp.Required()),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				if err := s.app.Decision.ResolveBlocker(ctx, argString(args, "blocker_id")); err != nil {
					return nil, err
				}
				return map[string]string{"status": "resolved"}, nil
			},
		},
		{
			name: "event.append",
			desc: "Append a canonical event (file.changed, test.failed, command.executed, ...).",
			options: []mcp.ToolOption{
				mcp.WithString("type", mcp.Description("Canonical event type"), mcp.Required()),
				mcp.WithString("payload", mcp.Description("Optional JSON payload")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				eventType := argString(args, "type")
				if err := s.app.Artifact.AppendEvent(ctx, sessionID, s.agent(), eventType, argString(args, "payload")); err != nil {
					return nil, err
				}
				if eventType == entities.EventTestPassed {
					return map[string]string{"status": "ok"}, s.maybeCheckpoint(ctx, sessionID, "tests passed")
				}
				return map[string]string{"status": "ok"}, nil
			},
		},
		{
			name: "workspace.status",
			desc: "Get git workspace status (branch, commit, dirty, changes).",
			run: func(ctx context.Context, args map[string]any) (any, error) {
				return s.app.Workspace.Status(ctx, s.app.Root)
			},
		},
		{
			name: "workspace.diff",
			desc: "Get the current git diff.",
			options: []mcp.ToolOption{
				mcp.WithString("scope", mcp.Description("stat|full")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				return s.app.Workspace.Diff(ctx, s.app.Root)
			},
		},
	}

	for _, spec := range specs {
		spec := spec
		s.mcp.AddTool(mcp.NewTool(spec.name, spec.options...), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := s.ready(); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result, err := spec.run(ctx, req.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, merr := json.Marshal(result)
			if merr != nil {
				return mcp.NewToolResultError(merr.Error()), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		})
	}
}

func (s *Server) runContextUpdate(ctx context.Context, args map[string]any) (any, error) {
	sessionID, err := s.currentSession(ctx)
	if err != nil {
		return nil, err
	}
	field := argString(args, "field")
	value := argString(args, "value")

	session, err := s.app.Session.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	switch field {
	case "task_title", "task_status":
		task, terr := s.app.Task.GetCurrent(ctx, sessionID)
		if terr != nil {
			return nil, terr
		}
		if field == "task_title" {
			_, terr = s.app.Task.Update(ctx, task.ID, value, "", s.agent())
		} else {
			_, terr = s.app.Task.Update(ctx, task.ID, "", value, s.agent())
		}
		return task, terr
	case "next_action":
		return s.app.Checkpoint.Create(ctx, sessionID, "", value, s.agent())
	case "session_title":
		session.Title = value
		if err := s.app.Store.Sessions().Update(ctx, session); err != nil {
			return nil, err
		}
		return session, nil
	default:
		return nil, fmt.Errorf("unknown field %q", field)
	}
}
