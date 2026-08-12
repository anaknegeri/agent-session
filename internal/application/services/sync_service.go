package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

// StaleThreshold is how many minutes without a checkpoint before auto-checkpoint fires.
const StaleThreshold = 10

// SyncService auto-records detectable changes (file modifications via git) so
// agents don't have to manually call event.append for every file they edit.
type SyncService struct {
	store    ports.Store
	git      ports.GitService
	artifact *ArtifactService
	logger   *slog.Logger
}

func NewSyncService(store ports.Store, git ports.GitService, artifact *ArtifactService, logger *slog.Logger) *SyncService {
	return &SyncService{store: store, git: git, artifact: artifact, logger: logger}
}

// SyncFileChanges detects files modified since the last recorded file.changed
// event and auto-appends events for any that haven't been recorded yet.
func (s *SyncService) SyncFileChanges(ctx context.Context, sessionID, projectPath, agent string) (int, error) {
	status, err := s.git.Status(ctx, projectPath)
	if err != nil || len(status.Changes) == 0 {
		return 0, nil
	}

	currentFiles := make(map[string]bool, len(status.Changes))
	for _, c := range status.Changes {
		currentFiles[c.Path] = true
	}

	recentEvents, err := s.store.Events().ListBySession(ctx, sessionID, 100)
	if err != nil {
		return 0, err
	}
	for _, e := range recentEvents {
		if e.Type != entities.EventFileChanged {
			continue
		}
		var payload struct {
			Files []string `json:"files"`
			Path  string   `json:"path"`
		}
		if e.Payload != "" {
			json.Unmarshal([]byte(e.Payload), &payload)
		}
		if payload.Path != "" {
			delete(currentFiles, payload.Path)
		}
		for _, f := range payload.Files {
			delete(currentFiles, f)
		}
	}

	if len(currentFiles) == 0 {
		return 0, nil
	}

	files := make([]string, 0, len(currentFiles))
	for f := range currentFiles {
		files = append(files, f)
	}

	payload, _ := json.Marshal(map[string]any{"files": files})
	if err := s.store.Events().Append(ctx, &entities.SessionEvent{
		ID:        ids.New("evt"),
		SessionID: sessionID,
		Agent:     agent,
		Type:      entities.EventFileChanged,
		Payload:   string(payload),
	}); err != nil {
		return 0, err
	}

	s.logger.Info("auto-recorded file changes", "session_id", sessionID, "files", len(files))
	return len(files), nil
}

// MinutesSinceCheckpoint returns minutes since the last checkpoint, or -1.
func (s *SyncService) MinutesSinceCheckpoint(ctx context.Context, sessionID string) int {
	cp, err := s.store.Checkpoints().GetLatest(ctx, sessionID)
	if err != nil || cp.CreatedAt.IsZero() {
		return -1
	}
	return int(time.Since(cp.CreatedAt.Time.UTC()).Minutes())
}

// IsStale reports whether the session should be auto-checkpointed.
func (s *SyncService) IsStale(ctx context.Context, sessionID, projectPath string) bool {
	minutes := s.MinutesSinceCheckpoint(ctx, sessionID)
	if minutes >= 0 && minutes < StaleThreshold {
		return false
	}
	status, err := s.git.Status(ctx, projectPath)
	if err != nil {
		return false
	}
	return status.Dirty
}

// UnrecordedFileCount returns the number of git-modified files that haven't
// been recorded as file.changed events yet.
func (s *SyncService) UnrecordedFileCount(ctx context.Context, sessionID, projectPath string) int {
	status, err := s.git.Status(ctx, projectPath)
	if err != nil || len(status.Changes) == 0 {
		return 0
	}

	currentFiles := make(map[string]bool, len(status.Changes))
	for _, c := range status.Changes {
		currentFiles[c.Path] = true
	}

	recentEvents, _ := s.store.Events().ListBySession(ctx, sessionID, 100)
	for _, e := range recentEvents {
		if e.Type != entities.EventFileChanged {
			continue
		}
		var payload struct {
			Files []string `json:"files"`
			Path  string   `json:"path"`
		}
		if e.Payload != "" {
			json.Unmarshal([]byte(e.Payload), &payload)
		}
		if payload.Path != "" {
			delete(currentFiles, payload.Path)
		}
		for _, f := range payload.Files {
			delete(currentFiles, f)
		}
	}
	return len(currentFiles)
}
