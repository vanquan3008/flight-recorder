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
	writes    sync.Mutex
	revision  uint64
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
	key := "ingest_token:" + tokenID
	for ctx.Err() == nil {
		now := cache.now()
		cache.mu.RLock()
		entry, found := cache.entries[tokenID]
		revision := cache.revision
		cache.mu.RUnlock()
		if found && now.Sub(entry.loadedAt) < cache.ttl {
			cache.observe("hit")
			return entry.token, entry.err
		}
		var token IngestToken
		if cache.byteCache != nil {
			raw, found, err := cache.byteCache.Get(ctx, key)
			if err == nil && found && json.Unmarshal(raw, &token) == nil {
				if !cache.publish(ctx, tokenID, revision, cachedTokenEntry{token: token, loadedAt: now}, false) {
					continue
				}
				cache.observe("hit")
				return token, nil
			}
		}
		cache.observe("miss")
		token, err := cache.source.GetIngestToken(ctx, tokenID)
		if err != nil && !errors.Is(err, ErrIngestTokenNotFound) {
			return token, err
		}
		if cache.publish(ctx, tokenID, revision, cachedTokenEntry{token: token, err: err, loadedAt: now}, err == nil) {
			return token, err
		}
	}
	return IngestToken{}, ctx.Err()
}

func (cache *CachedTokenSource) observe(result string) {
	if cache.observer != nil {
		cache.observer.ObserveAuthCache("token", result)
	}
}

// Serialize cache writes with invalidation while allowing local hits during Redis I/O.
func (cache *CachedTokenSource) publish(ctx context.Context, tokenID string, revision uint64, entry cachedTokenEntry, share bool) bool {
	cache.writes.Lock()
	defer cache.writes.Unlock()
	cache.mu.Lock()
	if cache.revision != revision {
		cache.mu.Unlock()
		return false
	}
	if len(cache.entries) >= cache.limit {
		cache.entries = make(map[string]cachedTokenEntry)
	}
	cache.entries[tokenID] = entry
	cache.mu.Unlock()
	if share && cache.byteCache != nil {
		if encoded, err := json.Marshal(entry.token); err == nil {
			_ = cache.byteCache.Set(ctx, "ingest_token:"+tokenID, encoded, cache.ttl)
		}
	}
	return true
}

// Invalidate removes a token from both caches and expires lookups already in flight.
func (cache *CachedTokenSource) Invalidate(tokenID string) {
	cache.writes.Lock()
	defer cache.writes.Unlock()
	if cache.byteCache != nil {
		_ = cache.byteCache.Delete(context.Background(), "ingest_token:"+tokenID)
	}
	cache.mu.Lock()
	cache.revision++
	delete(cache.entries, tokenID)
	cache.mu.Unlock()
}
