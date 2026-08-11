package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"gorm.io/gorm"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
	"github.com/anaknegeri/agent-session/internal/domain/repositories"
)

type projectStore struct {
	db *gorm.DB
}

func (s *projectStore) Create(ctx context.Context, project *entities.Project) error {
	if project.CreatedAt.IsZero() {
		project.CreatedAt = entities.Now()
	}
	if err := s.db.WithContext(ctx).Create(project).Error; err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (s *projectStore) GetByPath(ctx context.Context, path string) (*entities.Project, error) {
	p, err := s.getByPathExact(ctx, path)
	if err == nil || !errors.Is(err, domainerr.ErrProjectNotFound) {
		return p, err
	}
	// Fallback: projects initialized before path canonicalization may be stored
	// under a symlinked path (e.g. /var vs /private/var on macOS).
	resolved, rerr := filepath.EvalSymlinks(path)
	if rerr != nil || resolved == path {
		return nil, err
	}
	return s.getByPathExact(ctx, resolved)
}

func (s *projectStore) getByPathExact(ctx context.Context, path string) (*entities.Project, error) {
	var p entities.Project
	err := s.db.WithContext(ctx).First(&p, "path = ?", path).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrProjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by path: %w", err)
	}
	return &p, nil
}

func (s *projectStore) GetByID(ctx context.Context, id string) (*entities.Project, error) {
	var p entities.Project
	err := s.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainerr.ErrProjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by id: %w", err)
	}
	return &p, nil
}

var _ repositories.ProjectRepository = (*projectStore)(nil)
