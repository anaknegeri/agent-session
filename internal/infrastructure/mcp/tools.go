package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

type toolSpec struct {
	name        string
	desc        string
	options     []mcp.ToolOption
	readOnly    bool
	destructive bool
	idempotent  bool
	run         func(ctx context.Context, args map[string]any) (any, error)
}

type toolFlagSpec struct {
	readOnly    bool
	destructive bool
	idempotent  bool
}

// toolFlags annotate tools with MCP behavior hints so agents can pick the right
// tool without trial-and-error (saves tokens).
var toolFlags = map[string]toolFlagSpec{
	"session.get":        {readOnly: true},
	"session.diff":       {readOnly: true},
	"context.get":        {idempotent: true}, // auto-syncs file changes and may auto-checkpoint — not read-only
	"context.read":       {readOnly: true},
	"context.summarize":  {readOnly: true},
	"session.record":     {idempotent: true},
	"task.get":           {readOnly: true},
	"decision.list":      {readOnly: true},
	"blocker.list":       {readOnly: true},
	"workspace.status":   {readOnly: true},
	"workspace.diff":     {readOnly: true},
	"memory.get":         {readOnly: true},
	"memory.search":      {readOnly: true},
	"session.checkpoint": {idempotent: true},
	"session.resume":     {idempotent: true},
	"task.create":        {idempotent: true},
	"task.update":        {idempotent: true},
	"decision.create":    {idempotent: true},
	"blocker.create":     {idempotent: true},
	"blocker.resolve":    {idempotent: true},
	"event.append":       {idempotent: true},
	"context.update":     {idempotent: true},
	"memory.put":         {idempotent: true},
	"memory.promote":     {idempotent: true},
	"memory.delete":      {destructive: true},
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
			name: "session.diff",
			desc: "Show what changed between two checkpoints (defaults to the two latest).",
			options: []mcp.ToolOption{
				mcp.WithString("before_id", mcp.Description("Before checkpoint ID (defaults to second-latest)")),
				mcp.WithString("after_id", mcp.Description("After checkpoint ID (defaults to latest)")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				beforeID := argString(args, "before_id")
				afterID := argString(args, "after_id")
				if beforeID == "" || afterID == "" {
					cps, cerr := s.app.Checkpoint.ListBySession(ctx, sessionID, 10)
					if cerr != nil {
						return nil, cerr
					}
					if len(cps) < 2 {
						return nil, fmt.Errorf("need at least two checkpoints to diff")
					}
					if beforeID == "" {
						beforeID = cps[1].ID
					}
					if afterID == "" {
						afterID = cps[0].ID
					}
				}
				return s.app.Checkpoint.Diff(ctx, beforeID, afterID)
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
			name: "context.read",
			desc: "Read the session context without recording anything. Same output as context.get, but skips the file-change sync and the auto-checkpoint. Use this when the client runs read-only; prefer context.get otherwise so file changes stay recorded.",
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
				return s.app.Context.Read(ctx, sessionID, depth)
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
			name: "context.summarize",
			desc: "Ask the agent to summarize the session and store it via memory.put (uses the agent's own model, no external LLM).",
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				// Read, not Get: this tool is annotated read-only, and Get syncs file
				// changes and may auto-checkpoint. A client that trusts the annotation
				// would have been writing to the session while believing it was not.
				contextText, err := s.app.Context.Read(ctx, sessionID, "summary")
				if err != nil {
					return nil, err
				}
				events, _ := s.app.Event.List(ctx, sessionID, 8)

				var b strings.Builder
				b.WriteString("Write a concise summary of this coding session (2-4 sentences) covering: current task, progress, key decisions, blockers, and next action. ")
				b.WriteString("Then store the summary using the memory.put tool with kind=project_knowledge. ")
				b.WriteString("Base the summary ONLY on the data below.\n\n")
				b.WriteString(contextText)
				if len(events) > 0 {
					b.WriteString("\n\n## Recent events\n")
					for _, e := range events {
						fmt.Fprintf(&b, "- %s [%s]\n", e.Type, e.Agent)
					}
				}
				return b.String(), nil
			},
		},
		{
			name: "session.record",
			desc: "Record work in one call: append an event, log a decision, set next_action, and optionally checkpoint. Prefer this over several separate calls.",
			options: []mcp.ToolOption{
				mcp.WithString("event_type", mcp.Description("Canonical event type to append (e.g. file.changed, test.passed, command.executed)")),
				mcp.WithString("event_payload", mcp.Description("Optional JSON payload for the event")),
				mcp.WithString("decision", mcp.Description("Decision to record (with reason)")),
				mcp.WithString("decision_reason", mcp.Description("Reason behind the decision")),
				mcp.WithString("next_action", mcp.Description("Next action to store on the checkpoint")),
				mcp.WithBoolean("checkpoint", mcp.Description("Create a checkpoint after recording (default false)")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				agent := s.agent()
				results := map[string]any{}

				if et := argString(args, "event_type"); et != "" {
					if err := s.app.Artifact.AppendEvent(ctx, sessionID, agent, et, argString(args, "event_payload")); err != nil {
						return nil, err
					}
					results["event"] = et
				}

				if d := argString(args, "decision"); d != "" {
					dec, err := s.app.Decision.Create(ctx, sessionID, d, argString(args, "decision_reason"), agent)
					if err != nil {
						return nil, err
					}
					results["decision_id"] = dec.ID
				}

				if argBool(args, "checkpoint") {
					nextAction := argString(args, "next_action")
					cp, err := s.app.Checkpoint.Create(ctx, sessionID, "record", nextAction, agent)
					if err != nil {
						return nil, err
					}
					results["checkpoint_id"] = cp.ID
					_, _ = s.app.Context.WriteContextMD(ctx, s.app.Root, sessionID)
				}

				if len(results) == 0 {
					return nil, fmt.Errorf("session.record needs at least one of: event_type, decision, checkpoint=true")
				}
				return results, nil
			},
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
			name: "memory.put",
			desc: "Store a piece of long-term knowledge (kind: project_knowledge, architecture, solution, preference, skill).",
			options: []mcp.ToolOption{
				mcp.WithString("kind", mcp.Description("project_knowledge|architecture|solution|preference|skill"), mcp.Required()),
				mcp.WithString("content", mcp.Description("The knowledge to remember"), mcp.Required()),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, _ := s.currentSession(ctx)
				return s.app.Memory.Put(ctx, sessionID, argString(args, "kind"), argString(args, "content"), s.agent())
			},
		},
		{
			name: "memory.get",
			desc: "Get a knowledge entry by ID.",
			options: []mcp.ToolOption{
				mcp.WithString("memory_id", mcp.Description("Knowledge ID"), mcp.Required()),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				return s.app.Memory.Get(ctx, argString(args, "memory_id"))
			},
		},
		{
			name: "memory.search",
			desc: "Full-text search the knowledge store.",
			options: []mcp.ToolOption{
				mcp.WithString("query", mcp.Description("Search query"), mcp.Required()),
				mcp.WithNumber("limit", mcp.Description("Max results")),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				return s.app.Memory.Search(ctx, argString(args, "query"), intArg(args, "limit"))
			},
		},
		{
			name: "memory.delete",
			desc: "Delete a knowledge entry by ID.",
			options: []mcp.ToolOption{
				mcp.WithString("memory_id", mcp.Description("Knowledge ID"), mcp.Required()),
			},
			run: func(ctx context.Context, args map[string]any) (any, error) {
				if err := s.app.Memory.Delete(ctx, argString(args, "memory_id")); err != nil {
					return nil, err
				}
				return map[string]string{"status": "deleted"}, nil
			},
		},
		{
			name: "memory.promote",
			desc: "Promote session decisions, resolved blockers and completed tasks into long-term memory.",
			run: func(ctx context.Context, args map[string]any) (any, error) {
				sessionID, err := s.currentSession(ctx)
				if err != nil {
					return nil, err
				}
				count, err := s.app.Memory.Promote(ctx, sessionID, s.agent())
				if err != nil {
					return nil, err
				}
				return map[string]any{"promoted": count}, nil
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
		if f, ok := toolFlags[spec.name]; ok {
			spec.readOnly, spec.destructive, spec.idempotent = f.readOnly, f.destructive, f.idempotent
		}
		annotation := mcp.ToolAnnotation{}
		if spec.readOnly {
			annotation.ReadOnlyHint = mcp.ToBoolPtr(true)
		}
		if spec.destructive {
			annotation.DestructiveHint = mcp.ToBoolPtr(true)
		}
		if spec.idempotent {
			annotation.IdempotentHint = mcp.ToBoolPtr(true)
		}
		toolOpts := append(spec.options, mcp.WithToolAnnotation(annotation))
		s.mcp.AddTool(mcp.NewTool(spec.name, toolOpts...), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
