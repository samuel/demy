package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/samuel/demy/internal/model"
)

// SeedDefaultLibraryPlugins ensures every built-in plugin exists in the library,
// creating any that are missing. It is idempotent and matches on slug, so it
// adds newly-introduced built-ins to an already-populated library while leaving
// existing entries untouched.
func (s *Service) SeedDefaultLibraryPlugins(ctx context.Context) error {
	existing, err := s.store.ListLibraryPlugins(ctx, false)
	if err != nil {
		return fmt.Errorf("seed library: %w", err)
	}
	haveSlug := make(map[string]bool, len(existing))
	for _, p := range existing {
		haveSlug[p.Slug] = true
	}

	now := time.Now()
	var added int
	for _, p := range defaultLibraryPlugins(now) {
		if haveSlug[p.Slug] {
			continue
		}
		if err := s.store.CreateLibraryPlugin(ctx, p); err != nil {
			return fmt.Errorf("seed library plugin %q: %w", p.Slug, err)
		}
		added++
	}
	if added > 0 {
		slog.Info("seeded default library plugins", "count", added)
	}
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
			ID:          uuid.New().String(),
			Slug:        "comic",
			Name:        "Daily Comic",
			Description: "Latest daily comic strip from a comiccaster.xyz feed.",
			PluginType:  model.PluginTypeBuiltin,
			ConfigSchema: `{
  "type": "object",
  "properties": {
    "slug": {"type": "string", "title": "Comic feed slug", "description": "comiccaster.xyz feed slug, e.g. farside-daily"}
  },
  "required": ["slug"]
}`,
			IsActive:  true,
			SortOrder: 4,
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
			SortOrder:    5,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}
