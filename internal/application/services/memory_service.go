package services

import (
	"context"
	"log/slog"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

// MemoryService manages the long-term knowledge store (PRD §26, §36).
// Memory is deliberately separate from session state.
type MemoryService struct {
	store  ports.Store
	logger *slog.Logger
}

func NewMemoryService(store ports.Store, logger *slog.Logger) *MemoryService {
	return &MemoryService{store: store, logger: logger}
}

// Put records a piece of knowledge. sessionID may be empty for project-level
// knowledge.
func (s *MemoryService) Put(ctx context.Context, sessionID, kind, content, agent string) (*entities.Knowledge, error) {
	k := &entities.Knowledge{
		ID:         ids.New("mem"),
		SessionID:  sessionID,
		Kind:       kind,
		Content:    content,
		SourceType: entities.KnowledgeSourceManual,
		Agent:      agent,
	}
	if err := s.store.Knowledge().Create(ctx, k); err != nil {
		return nil, err
	}
	s.logger.Info("knowledge stored", "id", k.ID, "kind", kind)
	return k, nil
}

func (s *MemoryService) Get(ctx context.Context, id string) (*entities.Knowledge, error) {
	return s.store.Knowledge().GetByID(ctx, id)
}

func (s *MemoryService) Search(ctx context.Context, query string, limit int) ([]*entities.KnowledgeHit, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.store.Knowledge().Search(ctx, query, limit)
}

func (s *MemoryService) ListByKind(ctx context.Context, kind string, limit int) ([]*entities.Knowledge, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.Knowledge().ListByKind(ctx, kind, limit)
}

func (s *MemoryService) Delete(ctx context.Context, id string) error {
	return s.store.Knowledge().Delete(ctx, id)
}

// Promote auto-extracts structured session state into long-term memory
// (PRD §36, non-LLM):
//   - decisions             -> architecture
//   - resolved blockers     -> project_knowledge
//   - completed tasks       -> solution
//
// It is idempotent: an entity already promoted is skipped.
func (s *MemoryService) Promote(ctx context.Context, sessionID, agent string) (int, error) {
	repo := s.store.Knowledge()
	promoted := 0

	add := func(sourceType, sourceID, kind, content, srcAgent string) error {
		if sourceID == "" {
			return nil
		}
		exists, err := repo.ExistsSource(ctx, sourceType, sourceID)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		k := &entities.Knowledge{
			ID:         ids.New("mem"),
			SessionID:  sessionID,
			Kind:       kind,
			Content:    content,
			SourceType: sourceType,
			SourceID:   sourceID,
			Agent:      srcAgent,
		}
		if err := repo.Create(ctx, k); err != nil {
			return err
		}
		promoted++
		return nil
	}

	decisions, err := s.store.Decisions().ListBySession(ctx, sessionID)
	if err != nil {
		return promoted, err
	}
	for _, d := range decisions {
		content := d.Decision
		if d.Reason != "" {
			content = d.Decision + " — reason: " + d.Reason
		}
		if err := add(entities.KnowledgeSourceDecision, d.ID, entities.KnowledgeKindArchitecture, content, d.Agent); err != nil {
			return promoted, err
		}
	}

	blockers, err := s.store.Blockers().ListBySession(ctx, sessionID)
	if err != nil {
		return promoted, err
	}
	for _, b := range blockers {
		if b.Status != entities.BlockerStatusResolved {
			continue
		}
		content := b.Description
		if b.Agent != "" {
			content = b.Description + " (resolved)"
		}
		if err := add(entities.KnowledgeSourceBlocker, b.ID, entities.KnowledgeKindProject, content, b.Agent); err != nil {
			return promoted, err
		}
	}

	tasks, err := s.store.Tasks().ListBySession(ctx, sessionID)
	if err != nil {
		return promoted, err
	}
	for _, t := range tasks {
		if t.Status != entities.TaskStatusCompleted {
			continue
		}
		if err := add(entities.KnowledgeSourceTask, t.ID, entities.KnowledgeKindSolution, t.Title, agent); err != nil {
			return promoted, err
		}
	}

	if promoted > 0 {
		s.logger.Info("memory promoted", "session_id", sessionID, "count", promoted)
	}
	return promoted, nil
}
