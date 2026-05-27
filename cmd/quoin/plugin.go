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

// PNG returns cached PNG bytes or calls plugin.Render if the cache is stale.
// On render error it serves stale bytes (if any) and logs a warning.
func (sc *screenCache) PNG(ctx context.Context, cfg demy.Config) ([]byte, error) {
	sc.mu.Lock()
	stale := len(sc.pngBytes) == 0 ||
		time.Since(sc.renderedAt) > sc.ttl ||
		sc.lastW != cfg.PixelWidth || sc.lastH != cfg.PixelHeight
	sc.mu.Unlock()

	if !stale {
		sc.mu.Lock()
		b := sc.pngBytes
		sc.mu.Unlock()
		return b, nil
	}

	// Render outside the lock. If two goroutines both observe stale,
	// both render and the last writer wins — renders are idempotent.
	b, err := sc.plugin.Render(ctx, cfg)
	if err != nil {
		sc.mu.Lock()
		stale := len(sc.pngBytes) == 0
		b = sc.pngBytes
		sc.mu.Unlock()
		if stale {
			return nil, err
		}
		slog.Warn("plugin render failed, serving stale", "err", err)
		return b, nil
	}

	sc.mu.Lock()
	sc.pngBytes = b
	sc.renderedAt = time.Now()
	sc.lastW = cfg.PixelWidth
	sc.lastH = cfg.PixelHeight
	sc.mu.Unlock()
	return b, nil
}

// renderedFilename returns a filename for the current cached render.
// It changes when the underlying PNG bytes are refreshed.
func (sc *screenCache) renderedFilename(screenName string) string {
	sc.mu.Lock()
	t := sc.renderedAt
	sc.mu.Unlock()
	return fmt.Sprintf("%s-%s.png", screenName, t.UTC().Format("20060102T150405Z"))
}
