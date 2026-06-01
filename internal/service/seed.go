package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/samuel/demy/internal/model"
)

// SeedDefaultLibraryPlugins populates the library with the built-in plugins
// when it is empty (first startup). It is idempotent: a non-empty library is
// left unchanged.
func (s *Service) SeedDefaultLibraryPlugins(ctx context.Context) error {
	existing, err := s.store.ListLibraryPlugins(ctx, false)
	if err != nil {
		return fmt.Errorf("seed library: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}

	now := time.Now()
	plugins := defaultLibraryPlugins(now)
	for _, p := range plugins {
		if err := s.store.CreateLibraryPlugin(ctx, p); err != nil {
			return fmt.Errorf("seed library plugin %q: %w", p.Slug, err)
		}
	}
	slog.Info("seeded default library plugins", "count", len(plugins))
	return nil
}

func defaultLibraryPlugins(now time.Time) []*model.LibraryPlugin {
	return []*model.LibraryPlugin{
		{
			ID:          uuid.New().String(),
			Slug:        "weather",
			Name:        "Weather",
			Description: "Current conditions and forecast from Home Assistant.",
			PluginType:  model.PluginTypeBuiltin,
			ConfigSchema: `{
  "type": "object",
  "properties": {
    "ha_url":    {"type": "string", "title": "Home Assistant URL"},
    "ha_token":  {"type": "string", "title": "Long-lived access token"},
    "entity":    {"type": "string", "title": "Weather entity", "default": "weather.home"},
    "location":  {"type": "string", "title": "Location name"},
    "units":     {"type": "string", "title": "Units", "enum": ["metric", "imperial"]}
  },
  "required": ["ha_url", "ha_token"]
}`,
			IsActive:  true,
			SortOrder: 1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           uuid.New().String(),
			Slug:         "xkcd",
			Name:         "XKCD",
			Description:  "Latest XKCD comic with alt-text.",
			PluginType:   model.PluginTypeBuiltin,
			ConfigSchema: `{}`,
			IsActive:     true,
			SortOrder:    2,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:          uuid.New().String(),
			Slug:        "calendar",
			Name:        "Calendar",
			Description: "Upcoming events from Home Assistant calendars.",
			PluginType:  model.PluginTypeBuiltin,
			ConfigSchema: `{
  "type": "object",
  "properties": {
    "ha_url":     {"type": "string", "title": "Home Assistant URL"},
    "ha_token":   {"type": "string", "title": "Long-lived access token"},
    "calendars":  {"type": "array", "items": {"type": "string"}, "title": "Calendar entity IDs"}
  },
  "required": ["ha_url", "ha_token"]
}`,
			IsActive:  true,
			SortOrder: 3,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           uuid.New().String(),
			Slug:         "grayscale",
			Name:         "Grayscale Test",
			Description:  "Grayscale palette test pattern.",
			PluginType:   model.PluginTypeBuiltin,
			ConfigSchema: `{}`,
			IsActive:     true,
			SortOrder:    4,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}
