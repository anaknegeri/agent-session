package store

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type knowledgeStore struct {
	db *gorm.DB
}

func (s *knowledgeStore) Create(ctx context.Context, k *entities.Knowledge) error {
	if k.CreatedAt.IsZero() {
		k.CreatedAt = entities.Now()
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(k).Error; err != nil {
			return fmt.Errorf("insert knowledge: %w", err)
		}
		// keep the standalone FTS5 index in sync
		if err := tx.Exec(
			"INSERT INTO knowledge_fts (content, id, kind) VALUES (?, ?, ?)",
			k.Content, k.ID, k.Kind,
		).Error; err != nil {
			return fmt.Errorf("insert knowledge_fts: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *knowledgeStore) GetByID(ctx context.Context, id string) (*entities.Knowledge, error) {
	var k entities.Knowledge
	if err := s.db.WithContext(ctx).First(&k, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("get knowledge: %w", err)
	}
	return &k, nil
}

func (s *knowledgeStore) ListByKind(ctx context.Context, kind string, limit int) ([]*entities.Knowledge, error) {
	var rows []*entities.Knowledge
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list knowledge: %w", err)
	}
	return rows, nil
}

func (s *knowledgeStore) Search(ctx context.Context, query string, limit int) ([]*entities.KnowledgeHit, error) {
	match, err := ftsMatch(query, "AND")
	if err != nil {
		return nil, err
	}
	return s.search(ctx, match, limit)
}

// SearchAny matches any term (OR), ranked by bm25 — used to surface related
// knowledge when not all terms are present.
func (s *knowledgeStore) SearchAny(ctx context.Context, query string, limit int) ([]*entities.KnowledgeHit, error) {
	match, err := ftsMatch(query, "OR")
	if err != nil {
		return nil, err
	}
	return s.search(ctx, match, limit)
}

func (s *knowledgeStore) search(ctx context.Context, match string, limit int) ([]*entities.KnowledgeHit, error) {
	sql := `SELECT k.id, k.session_id, k.kind, k.content, k.source_type, k.source_id, k.agent, k.created_at,
	        snippet(knowledge_fts, 0, '«', '»', '…', 12) AS snippet
	        FROM knowledge_fts
	        JOIN knowledge k ON k.id = knowledge_fts.id
	        WHERE knowledge_fts MATCH ?
	        ORDER BY bm25(knowledge_fts)
	        LIMIT ?`
	var hits []*entities.KnowledgeHit
	if err := s.db.WithContext(ctx).Raw(sql, match, limit).Scan(&hits).Error; err != nil {
		return nil, fmt.Errorf("search knowledge: %w", err)
	}
	return hits, nil
}

// ftsMatch turns a user query into an FTS5 query joined by the operator
// ("AND" or "OR") of quoted terms, so "refresh token" finds "refresh tokens".
func ftsMatch(query, operator string) (string, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return "", fmt.Errorf("empty search query")
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		parts = append(parts, `"`+term+`"`)
	}
	return strings.Join(parts, " "+operator+" "), nil
}

func (s *knowledgeStore) Delete(ctx context.Context, id string) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM knowledge_fts WHERE id = ?", id).Error; err != nil {
			return fmt.Errorf("delete knowledge_fts: %w", err)
		}
		if err := tx.Exec("DELETE FROM knowledge WHERE id = ?", id).Error; err != nil {
			return fmt.Errorf("delete knowledge: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *knowledgeStore) ExistsSource(ctx context.Context, sourceType, sourceID string) (bool, error) {
	if sourceType == "" || sourceID == "" {
		return false, nil
	}
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&entities.Knowledge{}).
		Where("source_type = ? AND source_id = ?", sourceType, sourceID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check knowledge source: %w", err)
	}
	return count > 0, nil
}

var _ repositories.KnowledgeRepository = (*knowledgeStore)(nil)
