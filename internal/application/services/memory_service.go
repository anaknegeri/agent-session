package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
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
//
// An unknown kind is refused. knowledge kind is a frozen vocabulary
// (format-v1.md) and nothing enforced it, so `memory.put kind=note` stored a row
// that no kind-filtered read ever returns again: `memory list --kind`, the
// promotion sources and the context injection all select by kind. Storing it was
// worse than refusing it, because the caller was told it succeeded.
func (s *MemoryService) Put(ctx context.Context, sessionID, kind, content, agent string) (*entities.Knowledge, error) {
	if !entities.ValidKnowledgeKind(kind) {
		return nil, fmt.Errorf("%w %q, expected one of: %s",
			domainerr.ErrInvalidKnowledgeKind, kind, strings.Join(entities.KnowledgeKinds(), ", "))
	}
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
//
// The whole sweep is one transaction. It used to run N unguarded creates and
// return the count it had reached when one failed, so a caller was told "7
// promoted" over a database holding 7 rows it could not name — and a retry then
// re-walked the same source rows. Now it either promotes everything it found or
// nothing, and the count it reports is the count that committed.
func (s *MemoryService) Promote(ctx context.Context, sessionID, agent string) (int, error) {
	promoted := 0
	if err := s.store.Tx(ctx, func(st ports.Store) error {
		promoted = 0
		repo := st.Knowledge()

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

		decisions, err := st.Decisions().ListBySession(ctx, sessionID)
		if err != nil {
			return err
		}
		for _, d := range decisions {
			content := d.Decision
			if d.Reason != "" {
				content = d.Decision + " — reason: " + d.Reason
			}
			if err := add(entities.KnowledgeSourceDecision, d.ID, entities.KnowledgeKindArchitecture, content, d.Agent); err != nil {
				return err
			}
		}

		blockers, err := st.Blockers().ListBySession(ctx, sessionID)
		if err != nil {
			return err
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
				return err
			}
		}

		tasks, err := st.Tasks().ListBySession(ctx, sessionID)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if t.Status != entities.TaskStatusCompleted {
				continue
			}
			if err := add(entities.KnowledgeSourceTask, t.ID, entities.KnowledgeKindSolution, t.Title, agent); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}

	if promoted > 0 {
		s.logger.Info("memory promoted", "session_id", sessionID, "count", promoted)
	}
	return promoted, nil
}
