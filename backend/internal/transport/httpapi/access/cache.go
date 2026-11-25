package access

import (
	"sync"
	"time"
)

type cacheEntry[V any] struct {
	value    V
	loadedAt time.Time
}

// ttlCache spares the database repeated lookups of the same project, role or environment list within
// a short TTL. A full cache starts over instead of evicting entries one by one: every entry is cheap
// to reload, and a bounded map keeps memory flat however many projects callers name. A nil cache
// stores nothing, so every lookup goes to the database.
type ttlCache[V any] struct {
	mu      sync.Mutex
	entries map[string]cacheEntry[V]
	ttl     time.Duration
	limit   int
}

func newTTLCache[V any](ttl time.Duration, limit int) *ttlCache[V] {
	return &ttlCache[V]{entries: make(map[string]cacheEntry[V]), ttl: ttl, limit: limit}
}

func (cache *ttlCache[V]) get(key string, now time.Time) (V, bool) {
	var zero V
	if cache == nil {
		return zero, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, found := cache.entries[key]
	if !found || now.Sub(entry.loadedAt) >= cache.ttl {
		return zero, false
	}
	return entry.value, true
}

// put stores one value under every non-empty key, so a project is found by the reference the caller
// used as well as by its ID and slug.
func (cache *ttlCache[V]) put(value V, now time.Time, keys ...string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) >= cache.limit {
		cache.entries = make(map[string]cacheEntry[V])
	}
	for _, key := range keys {
		if key != "" {
			cache.entries[key] = cacheEntry[V]{value: value, loadedAt: now}
		}
	}
}

func (cache *ttlCache[V]) forget(keys ...string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, key := range keys {
		delete(cache.entries, key)
	}
}

func roleCacheKey(projectID, userID string) string {
	return projectID + ":" + userID
}
