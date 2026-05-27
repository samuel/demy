package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/samuel/demy"
)

// Plugin is instantiated once per screen definition at startup.
// Render fetches data, augments cfg with fonts, renders, and returns PNG bytes.
// Implementations must be safe for concurrent use.
type Plugin interface {
	Render(ctx context.Context, cfg demy.Config) ([]byte, error)
}

// screenCache wraps a Plugin with TTL-based caching keyed by pixel dimensions.
type screenCache struct {
	plugin Plugin
	ttl    time.Duration

	mu         sync.Mutex
	pngBytes   []byte
	renderedAt time.Time
	lastW      int
	lastH      int
}

func newScreenCache(p Plugin, ttl time.Duration) *screenCache {
	return &screenCache{plugin: p, ttl: ttl}
}

// PNG returns cached PNG bytes and the timestamp of the cached render, or calls
// plugin.Render if the cache is stale. The returned time.Time is the renderedAt
// that corresponds to the returned bytes — callers should derive filenames from
// it rather than calling a separate method to avoid TOCTOU races.
// On render error it serves stale bytes (if any) and logs a warning.
func (sc *screenCache) PNG(ctx context.Context, cfg demy.Config) ([]byte, time.Time, error) {
	sc.mu.Lock()
	stale := len(sc.pngBytes) == 0 ||
		time.Since(sc.renderedAt) > sc.ttl ||
		sc.lastW != cfg.PixelWidth || sc.lastH != cfg.PixelHeight
	if !stale {
		b, t := sc.pngBytes, sc.renderedAt
		sc.mu.Unlock()
		return b, t, nil
	}
	sc.mu.Unlock()

	// Render outside the lock. If two goroutines both observe stale,
	// both render and the last writer wins — renders are idempotent.
	b, err := sc.plugin.Render(ctx, cfg)
	if err != nil {
		sc.mu.Lock()
		stale := len(sc.pngBytes) == 0 ||
			sc.lastW != cfg.PixelWidth || sc.lastH != cfg.PixelHeight
		b, t := sc.pngBytes, sc.renderedAt
		sc.mu.Unlock()
		if stale {
			return nil, time.Time{}, err
		}
		slog.Warn("plugin render failed, serving stale", "err", err)
		return b, t, nil
	}

	t := time.Now()
	sc.mu.Lock()
	sc.pngBytes = b
	sc.renderedAt = t
	sc.lastW = cfg.PixelWidth
	sc.lastH = cfg.PixelHeight
	sc.mu.Unlock()
	return b, t, nil
}

// renderedFilename builds a stable filename for screenName from the given timestamp.
func renderedFilename(screenName string, t time.Time) string {
	return fmt.Sprintf("%s-%s.png", screenName, t.UTC().Format("20060102T150405Z"))
}
