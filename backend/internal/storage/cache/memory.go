package cache

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	value     []byte
	expiresAt time.Time
}

// MemoryCache provides a thread-safe in-memory cache bounded by maximum item limit and TTL.
type MemoryCache struct {
	mu         sync.RWMutex
	entries    map[string]memoryEntry
	limit      int
	defaultTTL time.Duration
	now        func() time.Time
}

// NewMemoryCache creates a new in-memory cache with specified default TTL and item capacity limit.
func NewMemoryCache(defaultTTL time.Duration, limit int) *MemoryCache {
	if defaultTTL <= 0 {
		defaultTTL = 60 * time.Second
	}
	if limit <= 0 {
		limit = 4096
	}
	return &MemoryCache{
		entries:    make(map[string]memoryEntry),
		limit:      limit,
		defaultTTL: defaultTTL,
		now:        time.Now,
	}
}

func (m *MemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	now := m.now()
	m.mu.RLock()
	entry, found := m.entries[key]
	m.mu.RUnlock()

	if !found {
		return nil, false, nil
	}

	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		m.mu.Lock()
		delete(m.entries, key)
		m.mu.Unlock()
		return nil, false, nil
	}

	result := make([]byte, len(entry.value))
	copy(result, entry.value)
	return result, true, nil
}

func (m *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	now := m.now()
	if ttl <= 0 {
		ttl = m.defaultTTL
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	valCopy := make([]byte, len(value))
	copy(valCopy, value)

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.entries) >= m.limit {
		// Evict expired entries first
		for k, e := range m.entries {
			if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
				delete(m.entries, k)
			}
		}
		// If still at limit, clear oldest partition to prevent unbounded memory growth
		if len(m.entries) >= m.limit {
			m.entries = make(map[string]memoryEntry, m.limit)
		}
	}

	m.entries[key] = memoryEntry{
		value:     valCopy,
		expiresAt: expiresAt,
	}
	return nil
}

func (m *MemoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
	return nil
}

func (m *MemoryCache) Close() error {
	m.mu.Lock()
	m.entries = make(map[string]memoryEntry)
	m.mu.Unlock()
	return nil
}

// NoopCache implements Cache with empty operations for tests or disabled caching.
type NoopCache struct{}

func NewNoopCache() *NoopCache { return &NoopCache{} }

func (n *NoopCache) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, nil
}

func (n *NoopCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

func (n *NoopCache) Delete(_ context.Context, _ string) error {
	return nil
}

func (n *NoopCache) Close() error {
	return nil
}
