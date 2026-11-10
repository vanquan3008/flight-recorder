package cache

import (
	"context"
	"time"
)

// NoopCache provides a stateless no-op implementation of Cache.
// Used when caching is disabled or in unit tests that bypass cache.
type NoopCache struct{}

// NewNoopCache constructs a ready-to-use NoopCache instance.
func NewNoopCache() *NoopCache {
	return &NoopCache{}
}

// Get always returns a cache miss without errors.
func (n *NoopCache) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, nil
}

// Set performs no operation and returns nil.
func (n *NoopCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// Delete performs no operation and returns nil.
func (n *NoopCache) Delete(_ context.Context, _ string) error {
	return nil
}

// Close performs no operation and returns nil.
func (n *NoopCache) Close() error {
	return nil
}
