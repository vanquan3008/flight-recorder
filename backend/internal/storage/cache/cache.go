package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

var (
	ErrKeyNotFound = errors.New("cache: key not found")
)

// Cache is a thread-safe caching abstraction supporting key-value storage with TTL.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Close() error
}

// New constructs a resilient cache based on configuration.
// If Redis is configured, it attempts to connect. If Redis fails or is unconfigured,
// it falls back to an in-memory cache with bounded limits.
func New(cfg config.CacheConfig, logger *slog.Logger) (Cache, error) {
	if logger == nil {
		logger = slog.Default()
	}

	driver := cfg.EffectiveDriver()
	if driver == config.CacheDriverNoop {
		return NewNoopCache(), nil
	}

	mem := NewMemoryCache(cfg.DefaultTTL, cfg.DefaultLimit)

	if driver == config.CacheDriverMemory {
		return mem, nil
	}

	// For Redis or Auto: attempt to construct Redis cache
	redisCache, err := NewRedisCache(cfg)
	if err != nil {
		logger.Warn("redis cache initialization failed; falling back to in-memory cache", "error", err)
		return mem, nil
	}

	// Test connection briefly with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if pingErr := redisCache.Ping(ctx); pingErr != nil {
		logger.Warn("redis ping failed; starting with in-memory fallback until redis is ready", "error", pingErr)
		return NewResilientCache(redisCache, mem, logger, false), nil
	}

	logger.Info("connected to redis cache successfully", "url", cfg.URL, "address", cfg.Address)
	return NewResilientCache(redisCache, mem, logger, true), nil
}
