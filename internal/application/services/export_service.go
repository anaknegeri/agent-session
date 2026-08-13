package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

type SessionExport struct {
	Session     *entities.Session        `json:"session"`
	Tasks       []*entities.Task         `json:"tasks"`
	Decisions   []*entities.Decision     `json:"decisions"`
	Blockers    []*entities.Blocker      `json:"blockers"`
	Events      []*entities.SessionEvent `json:"events"`
	Checkpoints []*entities.Checkpoint   `json:"checkpoints"`
	Memory      []*entities.Knowledge    `json:"memory"`
	ExportedAt  string                   `json:"exported_at"`
}

type ExportService struct {
	store ports.Store
}

func NewExportService(store ports.Store) *ExportService {
	return &ExportService{store: store}
}

func (s *ExportService) Export(ctx context.Context, sessionID string) (*SessionExport, error) {
	session, err := s.store.Sessions().GetByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	tasks, _ := s.store.Tasks().ListBySession(ctx, sessionID)
	decisions, _ := s.store.Decisions().ListBySession(ctx, sessionID)
	blockers, _ := s.store.Blockers().ListBySession(ctx, sessionID)
	events, _ := s.store.Events().ListBySession(ctx, sessionID, 500)
	checkpoints, _ := s.store.Checkpoints().ListBySession(ctx, sessionID, 500)
	memory, _ := s.store.Knowledge().ListByKind(ctx, "", 500)

	return &SessionExport{
		Session:     session,
		Tasks:       tasks,
		Decisions:   decisions,
		Blockers:    blockers,
		Events:      events,
		Checkpoints: checkpoints,
		Memory:      memory,
		ExportedAt:  time.Now().Format(time.RFC3339),
	}, nil
}

func (s *ExportService) ExportJSON(ctx context.Context, sessionID string) ([]byte, error) {
	exp, err := s.Export(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(exp, "", "  ")
}

func (s *ExportService) ExportMarkdown(ctx context.Context, sessionID string) (string, error) {
	exp, err := s.Export(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return renderExportMarkdown(exp), nil
}

func renderExportMarkdown(e *SessionExport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Session Export: %s\n\n", e.Session.Title)
	fmt.Fprintf(&b, "**ID:** %s  \n", e.Session.ID)
	fmt.Fprintf(&b, "**Status:** %s  \n", e.Session.Status)
	fmt.Fprintf(&b, "**Last Agent:** %s  \n", e.Session.LastAgent)
	fmt.Fprintf(&b, "**Exported:** %s\n\n", e.ExportedAt)

	if len(e.Tasks) > 0 {
		b.WriteString("## Tasks\n\n")
		for _, t := range e.Tasks {
			fmt.Fprintf(&b, "- [%s] %s\n", t.Status, t.Title)
		}
		b.WriteString("\n")
	}

	if len(e.Decisions) > 0 {
		b.WriteString("## Decisions\n\n")
		for _, d := range e.Decisions {
			line := d.Decision
			if d.Reason != "" {
				line += " — (" + d.Reason + ")"
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}

	if len(e.Blockers) > 0 {
		b.WriteString("## Blockers\n\n")
		for _, bl := range e.Blockers {
			marker := "✗"
			if bl.Status == entities.BlockerStatusResolved {
				marker = "✓"
			}
			fmt.Fprintf(&b, "- %s [%s] %s\n", marker, bl.Status, bl.Description)
		}
		b.WriteString("\n")
	}

	if len(e.Events) > 0 {
		b.WriteString("## Events\n\n")
		for _, ev := range e.Events {
			fmt.Fprintf(&b, "- %s %s [%s]\n", ev.CreatedAt.Format("2006-01-02 15:04"), ev.Type, ev.Agent)
		}
		b.WriteString("\n")
	}

	if len(e.Memory) > 0 {
		b.WriteString("## Memory\n\n")
		for _, m := range e.Memory {
			fmt.Fprintf(&b, "- [%s] %s\n", m.Kind, m.Content)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (s *ExportService) Import(ctx context.Context, projectID string, data []byte, agent string) (string, error) {
	var exp SessionExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return "", fmt.Errorf("parse export file: %w", err)
	}
	if exp.Session == nil {
		return "", fmt.Errorf("export file has no session data")
	}

	importedTitle := exp.Session.Title
	if importedTitle == "" {
		importedTitle = "Imported session"
	}
	importedTitle += " (imported)"

	session := &entities.Session{
		ID:        ids.New("sess"),
		ProjectID: projectID,
		Title:     importedTitle,
		Status:    entities.SessionStatusActive,
		LastAgent: agent,
	}
	if err := s.store.Sessions().Create(ctx, session); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	for _, t := range exp.Tasks {
		task := &entities.Task{
			ID:        ids.New("task"),
			SessionID: session.ID,
			Title:     t.Title,
			Status:    entities.TaskStatusInProgress,
		}
		_ = s.store.Tasks().Create(ctx, task)
		session.CurrentTaskID = task.ID
	}

	for _, d := range exp.Decisions {
		dec := &entities.Decision{
			ID:        ids.New("decision"),
			SessionID: session.ID,
			Decision:  d.Decision,
			Reason:    d.Reason,
			Agent:     agent,
		}
		_ = s.store.Decisions().Create(ctx, dec)
	}

	for _, bl := range exp.Blockers {
		if bl.Status != entities.BlockerStatusOpen {
			continue
		}
		blocker := &entities.Blocker{
			ID:          ids.New("blocker"),
			SessionID:   session.ID,
			Description: bl.Description,
			Status:      entities.BlockerStatusOpen,
			Agent:       agent,
		}
		_ = s.store.Blockers().Create(ctx, blocker)
	}

	_ = s.store.Sessions().Update(ctx, session)
	_ = s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: session.ID,
		Agent:     agent,
		Type:      entities.EventSessionStarted,
	})

	return session.ID, nil
}
