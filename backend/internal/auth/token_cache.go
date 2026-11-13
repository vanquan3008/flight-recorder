package auth

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type cachedTokenEntry struct {
	token    IngestToken
	err      error
	loadedAt time.Time
}

// ByteCache is an optional distributed or persistent cache interface (e.g. Redis).
type ByteCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// CacheObserver receives events when cache lookups hit or miss.
type CacheObserver interface {
	ObserveAuthCache(kind, result string)
}

// CachedTokenSource wraps an IngestTokenSource with a thread-safe in-memory cache
// and an optional distributed cache (Redis) with automatic local fallback.
type CachedTokenSource struct {
	mu        sync.RWMutex
	source    IngestTokenSource
	byteCache ByteCache
	entries   map[string]cachedTokenEntry
	ttl       time.Duration
	limit     int
	now       func() time.Time
	observer  CacheObserver
}

// NewCachedTokenSource creates an IngestTokenSource cache with the given TTL and entry limit.
func NewCachedTokenSource(source IngestTokenSource, ttl time.Duration, limit int) *CachedTokenSource {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if limit <= 0 {
		limit = 4096
	}
	return &CachedTokenSource{
		source:  source,
		entries: make(map[string]cachedTokenEntry),
		ttl:     ttl,
		limit:   limit,
		now:     time.Now,
	}
}

// WithByteCache attaches an external or resilient cache (e.g. Redis with Memory fallback).
func (cache *CachedTokenSource) WithByteCache(bc ByteCache) *CachedTokenSource {
	cache.byteCache = bc
	return cache
}

// WithObserver attaches a metric observer to the token cache.
func (cache *CachedTokenSource) WithObserver(observer CacheObserver) *CachedTokenSource {
	cache.observer = observer
	return cache
}

// GetIngestToken returns the cached IngestToken if fresh; otherwise it queries the underlying source.
func (cache *CachedTokenSource) GetIngestToken(ctx context.Context, tokenID string) (IngestToken, error) {
	now := cache.now()

	// 1. Check in-memory map
	cache.mu.RLock()
	entry, found := cache.entries[tokenID]
	cache.mu.RUnlock()
	if found && now.Sub(entry.loadedAt) < cache.ttl {
		if cache.observer != nil {
			cache.observer.ObserveAuthCache("token", "hit")
		}
		return entry.token, entry.err
	}

	// 2. Check external / distributed cache (Redis) if configured
	if cache.byteCache != nil {
		raw, found, err := cache.byteCache.Get(ctx, "ingest_token:"+tokenID)
		if err == nil && found && len(raw) > 0 {
			var tok IngestToken
			if jsonErr := json.Unmarshal(raw, &tok); jsonErr == nil {
				if cache.observer != nil {
					cache.observer.ObserveAuthCache("token", "hit")
				}
				// Populate local memory map for fast sub-millisecond future hits
				cache.mu.Lock()
				if len(cache.entries) >= cache.limit {
					cache.entries = make(map[string]cachedTokenEntry)
				}
				cache.entries[tokenID] = cachedTokenEntry{
					token:    tok,
					err:      nil,
					loadedAt: now,
				}
				cache.mu.Unlock()
				return tok, nil
			}
		}
	}

	if cache.observer != nil {
		cache.observer.ObserveAuthCache("token", "miss")
	}

	// 3. Fall back to database query
	token, lookupError := cache.source.GetIngestToken(ctx, tokenID)
	// Cache successful lookups and ErrIngestTokenNotFound. Transient errors are not cached.
	if lookupError == nil || errors.Is(lookupError, ErrIngestTokenNotFound) {
		cache.mu.Lock()
		if len(cache.entries) >= cache.limit {
			cache.entries = make(map[string]cachedTokenEntry)
		}
		cache.entries[tokenID] = cachedTokenEntry{
			token:    token,
			err:      lookupError,
			loadedAt: now,
		}
		cache.mu.Unlock()

		// Populate distributed cache on success
		if lookupError == nil && cache.byteCache != nil {
			if encoded, err := json.Marshal(token); err == nil {
				_ = cache.byteCache.Set(ctx, "ingest_token:"+tokenID, encoded, cache.ttl)
			}
		}
	}
	return token, lookupError
}

// Invalidate removes a token from both in-memory and external cache so revocation takes immediate effect.
func (cache *CachedTokenSource) Invalidate(tokenID string) {
	cache.mu.Lock()
	delete(cache.entries, tokenID)
	cache.mu.Unlock()

	if cache.byteCache != nil {
		_ = cache.byteCache.Delete(context.Background(), "ingest_token:"+tokenID)
	}
}
