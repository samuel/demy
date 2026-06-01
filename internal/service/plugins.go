package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/samuel/demy/internal/model"
)

// CreateUserPlugin creates a user plugin instance, optionally cloned from a library plugin.
func (s *Service) CreateUserPlugin(ctx context.Context, userID, libraryPluginID string) (*model.UserPlugin, error) {
	now := time.Now().UTC()
	p := &model.UserPlugin{
		ID:              uuid.New().String(),
		UserID:          userID,
		LibraryPluginID: libraryPluginID,
		Config:          "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if libraryPluginID != "" {
		lib, err := s.store.GetLibraryPluginByID(ctx, libraryPluginID)
		if err != nil {
			return nil, fmt.Errorf("get library plugin: %w", err)
		}
		p.Name = lib.Name
		p.PluginType = lib.PluginType
		p.Template = lib.Template
		p.Script = lib.Script
	}

	if err := s.store.CreateUserPlugin(ctx, p); err != nil {
		return nil, fmt.Errorf("create user plugin: %w", err)
	}
	return p, nil
}

// externalPluginJSON is the JSON structure for externally-hosted plugins.
type externalPluginJSON struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	EndpointURL  string          `json:"endpoint_url"`
	Template     string          `json:"template"`
	Refresh      string          `json:"refresh"`
}

// ImportUserPlugin creates a user plugin from an external URL.
func (s *Service) ImportUserPlugin(ctx context.Context, userID, externalURL string) (*model.UserPlugin, error) {
	resp, err := http.Get(externalURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetch external plugin: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read external plugin body: %w", err)
	}

	var ext externalPluginJSON
	if err := json.Unmarshal(body, &ext); err != nil {
		return nil, fmt.Errorf("parse external plugin json: %w", err)
	}

	// Store endpoint_url in the config JSON.
	cfgMap := map[string]string{"endpoint_url": ext.EndpointURL}
	cfgJSON, err := json.Marshal(cfgMap)
	if err != nil {
		return nil, fmt.Errorf("marshal plugin config: %w", err)
	}

	now := time.Now().UTC()
	p := &model.UserPlugin{
		ID:          uuid.New().String(),
		UserID:      userID,
		ExternalURL: externalURL,
		Name:        ext.Name,
		PluginType:  model.PluginType(ext.Type),
		Config:      string(cfgJSON),
		Template:    ext.Template,
		Refresh:     ext.Refresh,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateUserPlugin(ctx, p); err != nil {
		return nil, fmt.Errorf("create user plugin: %w", err)
	}
	return p, nil
}

// GetUserPlugin fetches a user plugin by ID, verifying ownership.
func (s *Service) GetUserPlugin(ctx context.Context, userID, id string) (*model.UserPlugin, error) {
	p, err := s.store.GetUserPluginByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user plugin: %w", err)
	}
	if p.UserID != userID {
		return nil, ErrUnauthorized
	}
	return p, nil
}

// ListUserPlugins returns all plugins for a user.
func (s *Service) ListUserPlugins(ctx context.Context, userID string) ([]*model.UserPlugin, error) {
	plugins, err := s.store.ListUserPlugins(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user plugins: %w", err)
	}
	return plugins, nil
}

// UpdateUserPlugin saves config/template/script changes.
func (s *Service) UpdateUserPlugin(ctx context.Context, p *model.UserPlugin) error {
	p.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateUserPlugin(ctx, p); err != nil {
		return fmt.Errorf("update user plugin: %w", err)
	}
	return nil
}

// DeleteUserPlugin removes a user plugin.
func (s *Service) DeleteUserPlugin(ctx context.Context, userID, id string) error {
	p, err := s.store.GetUserPluginByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get user plugin: %w", err)
	}
	if p.UserID != userID {
		return ErrUnauthorized
	}
	if err := s.store.DeleteUserPlugin(ctx, id); err != nil {
		return fmt.Errorf("delete user plugin: %w", err)
	}
	return nil
}
