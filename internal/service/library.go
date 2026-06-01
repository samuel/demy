package service

import (
	"context"
	"fmt"

	"github.com/samuel/demy/internal/model"
)

// ListLibraryPlugins returns all library plugins (active only for non-admins).
func (s *Service) ListLibraryPlugins(ctx context.Context, activeOnly bool) ([]*model.LibraryPlugin, error) {
	plugins, err := s.store.ListLibraryPlugins(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list library plugins: %w", err)
	}
	return plugins, nil
}

// GetLibraryPlugin fetches a library plugin by slug.
func (s *Service) GetLibraryPlugin(ctx context.Context, slug string) (*model.LibraryPlugin, error) {
	p, err := s.store.GetLibraryPluginBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("get library plugin: %w", err)
	}
	return p, nil
}

// GetLibraryPluginByID fetches a library plugin by ID.
func (s *Service) GetLibraryPluginByID(ctx context.Context, id string) (*model.LibraryPlugin, error) {
	p, err := s.store.GetLibraryPluginByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get library plugin by id: %w", err)
	}
	return p, nil
}

// CreateLibraryPlugin creates a new library plugin (admin only — caller must check).
func (s *Service) CreateLibraryPlugin(ctx context.Context, p *model.LibraryPlugin) error {
	if err := s.store.CreateLibraryPlugin(ctx, p); err != nil {
		return fmt.Errorf("create library plugin: %w", err)
	}
	return nil
}

// UpdateLibraryPlugin updates a library plugin.
func (s *Service) UpdateLibraryPlugin(ctx context.Context, p *model.LibraryPlugin) error {
	if err := s.store.UpdateLibraryPlugin(ctx, p); err != nil {
		return fmt.Errorf("update library plugin: %w", err)
	}
	return nil
}
