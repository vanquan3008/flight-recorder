package cache

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	value      []byte
	expiresAt  time.Time
	insertedAt time.Time
}

// MemoryCache provides a thread-safe, bounded in-memory cache with TTL support and capacity eviction.
type MemoryCache struct {
	mu         sync.RWMutex
	entries    map[string]memoryEntry
	limit      int
	defaultTTL time.Duration
	now        func() time.Time
	metrics    Metrics
}

// NewMemoryCache creates an in-memory cache with default TTL and capacity limits.
func NewMemoryCache(defaultTTL time.Duration, limit int) *MemoryCache {
	return NewMemoryCacheWithMetrics(defaultTTL, limit, nil)
}

// NewMemoryCacheWithMetrics creates an in-memory cache with an optional metrics collector.
func NewMemoryCacheWithMetrics(defaultTTL time.Duration, limit int, metrics Metrics) *MemoryCache {
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
		metrics:    metrics,
	}
}

func (m *MemoryCache) currentTime() time.Time {
	if m != nil && m.now != nil {
		return m.now()
	}
	return time.Now()
}

// Get retrieves the value associated with key.
// Returns a deep copy of the stored byte slice to ensure thread-safety against caller mutations.
func (m *MemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	now := m.currentTime()

	m.mu.RLock()
	entry, found := m.entries[key]
	m.mu.RUnlock()

	if !found {
		if m.metrics != nil {
			m.metrics.RecordMiss("memory")
		}
		return nil, false, nil
	}

	// Check TTL expiration
	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		m.mu.Lock()
		// Double check after acquiring write lock
		if e, ok := m.entries[key]; ok && !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			delete(m.entries, key)
			if m.metrics != nil {
				m.metrics.RecordEviction("memory", "expired")
			}
		}
		m.mu.Unlock()

		if m.metrics != nil {
			m.metrics.RecordMiss("memory")
		}
		return nil, false, nil
	}

	if m.metrics != nil {
		m.metrics.RecordHit("memory")
	}

	result := make([]byte, len(entry.value))
	copy(result, entry.value)
	return result, true, nil
}

// Set stores value for the key with specified TTL.
// Enforces bounded memory limits by evicting expired items or the oldest items when full.
func (m *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	now := m.currentTime()
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

	// If at capacity and adding a new key, perform eviction
	if _, exists := m.entries[key]; !exists && len(m.entries) >= m.limit {
		// Pass 1: Evict all expired entries first
		for k, e := range m.entries {
			if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
				delete(m.entries, k)
				if m.metrics != nil {
					m.metrics.RecordEviction("memory", "expired")
				}
			}
		}

		// Pass 2: If still at or above capacity, evict the oldest entry (FIFO)
		if len(m.entries) >= m.limit {
			var oldestKey string
			var oldestTime time.Time
			first := true

			for k, e := range m.entries {
				if first || e.insertedAt.Before(oldestTime) {
					oldestKey = k
					oldestTime = e.insertedAt
					first = false
				}
			}

			if oldestKey != "" {
				delete(m.entries, oldestKey)
				if m.metrics != nil {
					m.metrics.RecordEviction("memory", "capacity")
				}
			}
		}
	}

	m.entries[key] = memoryEntry{
		value:      valCopy,
		expiresAt:  expiresAt,
		insertedAt: now,
	}

	if m.metrics != nil {
		m.metrics.RecordSet("memory")
	}
	return nil
}

// Delete removes the specified key.
func (m *MemoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	_, found := m.entries[key]
	delete(m.entries, key)
	m.mu.Unlock()

	if found && m.metrics != nil {
		m.metrics.RecordDelete("memory")
	}
	return nil
}

// Len returns the current number of cached items.
func (m *MemoryCache) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// Close clears all cached entries.
func (m *MemoryCache) Close() error {
	m.mu.Lock()
	m.entries = make(map[string]memoryEntry)
	m.mu.Unlock()
	return nil
}
