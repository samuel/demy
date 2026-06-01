package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/samuel/demy/internal/model"
)

// StoreResourceCache wraps a model.Store to implement ResourceCache.
// Cache keys are SHA-256(userID + "|" + url) for per-user isolation.
type StoreResourceCache struct {
	store model.Store
}

// NewStoreResourceCache wraps store as a ResourceCache.
func NewStoreResourceCache(store model.Store) *StoreResourceCache {
	return &StoreResourceCache{store: store}
}

func cacheKey(userID, url string) string {
	h := sha256.Sum256([]byte(userID + "|" + url))
	return hex.EncodeToString(h[:])
}

// Get retrieves a cached resource. ok is false if not found or expired.
func (c *StoreResourceCache) Get(ctx context.Context, userID, url string) ([]byte, string, bool, error) {
	key := cacheKey(userID, url)
	r, err := c.store.GetCachedResource(ctx, key)
	if errors.Is(err, model.ErrNotFound) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("resource cache get: %w", err)
	}
	if time.Now().After(r.ExpiresAt) {
		return nil, "", false, nil
	}
	return r.Data, r.ContentType, true, nil
}

// Set stores a resource in the cache with the given TTL.
func (c *StoreResourceCache) Set(ctx context.Context, userID, url string, data []byte, contentType string, ttl time.Duration) error {
	now := time.Now()
	r := &model.CachedResource{
		Key:         cacheKey(userID, url),
		URL:         url,
		UserID:      userID,
		Data:        data,
		ContentType: contentType,
		FetchedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := c.store.SetCachedResource(ctx, r); err != nil {
		return fmt.Errorf("resource cache set: %w", err)
	}
	return nil
}
