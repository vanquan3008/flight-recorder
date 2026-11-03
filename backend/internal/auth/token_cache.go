package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

type cachedTokenEntry struct {
	token    IngestToken
	err      error
	loadedAt time.Time
}

// CacheObserver receives events when cache lookups hit or miss.
type CacheObserver interface {
	ObserveAuthCache(kind, result string)
}

// CachedTokenSource wraps an IngestTokenSource with a thread-safe in-memory cache to eliminate
// per-request database lookups during OTLP ingestion.
type CachedTokenSource struct {
	mu       sync.RWMutex
	source   IngestTokenSource
	entries  map[string]cachedTokenEntry
	ttl      time.Duration
	now      func() time.Time
	observer CacheObserver
}

// NewCachedTokenSource creates an IngestTokenSource cache with the given time-to-live.
func NewCachedTokenSource(source IngestTokenSource, ttl time.Duration) *CachedTokenSource {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedTokenSource{
		source:  source,
		entries: make(map[string]cachedTokenEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// WithObserver attaches a metric observer to the token cache.
func (cache *CachedTokenSource) WithObserver(observer CacheObserver) *CachedTokenSource {
	cache.observer = observer
	return cache
}

// GetIngestToken returns the cached IngestToken if fresh; otherwise it queries the underlying source.
func (cache *CachedTokenSource) GetIngestToken(ctx context.Context, tokenID string) (IngestToken, error) {
	now := cache.now()
	cache.mu.RLock()
	entry, found := cache.entries[tokenID]
	cache.mu.RUnlock()
	if found && now.Sub(entry.loadedAt) < cache.ttl {
		if cache.observer != nil {
			cache.observer.ObserveAuthCache("token", "hit")
		}
		return entry.token, entry.err
	}
	if cache.observer != nil {
		cache.observer.ObserveAuthCache("token", "miss")
	}

	token, lookupError := cache.source.GetIngestToken(ctx, tokenID)
	// Cache successful lookups and ErrIngestTokenNotFound. Transient errors are not cached.
	if lookupError == nil || errors.Is(lookupError, ErrIngestTokenNotFound) {
		cache.mu.Lock()
		if len(cache.entries) >= 4096 {
			cache.entries = make(map[string]cachedTokenEntry)
		}
		cache.entries[tokenID] = cachedTokenEntry{
			token:    token,
			err:      lookupError,
			loadedAt: now,
		}
		cache.mu.Unlock()
	}
	return token, lookupError
}

// Invalidate removes a token from the cache so revocation has immediate effect.
func (cache *CachedTokenSource) Invalidate(tokenID string) {
	cache.mu.Lock()
	delete(cache.entries, tokenID)
	cache.mu.Unlock()
}
