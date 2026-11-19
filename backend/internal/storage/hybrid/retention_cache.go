package hybrid

import (
	"context"
	"sync"
	"time"
)

// retentionCache spares PostgreSQL a policy read per telemetry request. Other
// replicas see an update within the TTL.
type retentionCache struct {
	ttl        time.Duration
	limit      int
	now        func() time.Time
	mu         sync.Mutex
	generation uint64
	entries    map[string]retentionEntry
}

type retentionEntry struct {
	days    *int32
	expires time.Time
}

func newRetentionCache(ttl time.Duration, limit int) *retentionCache {
	return &retentionCache{ttl: ttl, limit: max(limit, 1), now: time.Now, entries: map[string]retentionEntry{}}
}

// get does not cache a value loaded while forget ran.
func (cache *retentionCache) get(ctx context.Context, project string, load func(context.Context, string) (*int32, error)) (*int32, error) {
	if cache == nil || cache.ttl <= 0 {
		return load(ctx, project)
	}
	now := cache.now()
	cache.mu.Lock()
	entry, found := cache.entries[project]
	generation := cache.generation
	cache.mu.Unlock()
	if found && now.Before(entry.expires) {
		return copyDays(entry.days), nil
	}
	days, err := load(ctx, project)
	if err != nil {
		return nil, err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.generation == generation {
		if _, exists := cache.entries[project]; !exists && len(cache.entries) >= cache.limit {
			clear(cache.entries)
		}
		cache.entries[project] = retentionEntry{days: copyDays(days), expires: now.Add(cache.ttl)}
	}
	return days, nil
}

func (cache *retentionCache) forget(project string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.generation++
	delete(cache.entries, project)
}

func copyDays(days *int32) *int32 {
	if days == nil {
		return nil
	}
	value := *days
	return &value
}
