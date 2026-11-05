package cache

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// ResilientCache wraps a primary Redis cache with an in-memory fallback.
// If Redis is unavailable or returns an error during operations, it seamlessly
// degrades to the in-memory cache and logs warnings, ensuring system continuity.
type ResilientCache struct {
	redis     *RedisCache
	memory    *MemoryCache
	logger    *slog.Logger
	redisDown atomic.Bool
}

// NewResilientCache constructs a ResilientCache with the given primary and fallback stores.
func NewResilientCache(redis *RedisCache, memory *MemoryCache, logger *slog.Logger, redisHealthy bool) *ResilientCache {
	if logger == nil {
		logger = slog.Default()
	}
	rc := &ResilientCache{
		redis:  redis,
		memory: memory,
		logger: logger,
	}
	rc.redisDown.Store(!redisHealthy)
	return rc
}

func (r *ResilientCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if r.redis != nil && !r.redisDown.Load() {
		val, found, err := r.redis.Get(ctx, key)
		if err == nil {
			return val, found, nil
		}
		// Redis experienced an error: log warning, mark down, and fall back to memory
		if r.redisDown.CompareAndSwap(false, true) {
			r.logger.Warn("redis cache read failed; failing over to in-memory cache", "key", key, "error", err)
		}
	}

	return r.memory.Get(ctx, key)
}

func (r *ResilientCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Always populate in-memory fallback for warm cache in case of disconnect
	_ = r.memory.Set(ctx, key, value, ttl)

	if r.redis != nil && !r.redisDown.Load() {
		err := r.redis.Set(ctx, key, value, ttl)
		if err == nil {
			return nil
		}
		if r.redisDown.CompareAndSwap(false, true) {
			r.logger.Warn("redis cache write failed; storing in in-memory fallback", "key", key, "error", err)
		}
	}

	return nil
}

func (r *ResilientCache) Delete(ctx context.Context, key string) error {
	_ = r.memory.Delete(ctx, key)

	if r.redis != nil && !r.redisDown.Load() {
		err := r.redis.Delete(ctx, key)
		if err == nil {
			return nil
		}
		if r.redisDown.CompareAndSwap(false, true) {
			r.logger.Warn("redis cache delete failed; evicting from in-memory fallback", "key", key, "error", err)
		}
	}

	return nil
}

func (r *ResilientCache) Close() error {
	if r.redis != nil {
		_ = r.redis.Close()
	}
	return r.memory.Close()
}
