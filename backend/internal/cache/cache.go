package cache

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

const redisStartupTimeout = 2 * time.Second

// Cache defines a thread-safe key-value cache with TTL support.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Close() error
}

// New creates a cache using the configured driver.
func New(cfg config.CacheConfig, logger *slog.Logger) (Cache, error) {
	if logger == nil {
		logger = slog.Default()
	}

	switch driver := cfg.EffectiveDriver(); driver {
	case config.CacheDriverNoop:
		logger.Debug("cache initialized", "driver", "noop")
		return NewNoopCache(), nil

	case config.CacheDriverMemory:
		logger.Info("cache initialized", "driver", "memory")
		return newMemoryCache(cfg), nil

	case config.CacheDriverRedis, config.CacheDriverAuto:
		return newResilientRedisCache(cfg, logger)

	default:
		return nil, fmt.Errorf("unsupported cache driver: %q", driver)
	}
}

// newMemoryCache constructs a bounded in-memory cache.
func newMemoryCache(cfg config.CacheConfig) Cache {
	return NewMemoryCache(cfg.DefaultTTL, cfg.DefaultLimit)
}

// newResilientRedisCache initializes Redis with an in-memory fallback.
func newResilientRedisCache(cfg config.CacheConfig, logger *slog.Logger) (Cache, error) {
	redisCache, err := NewRedisCache(cfg)
	if err != nil {
		logger.Warn("redis initialization failed; using memory fallback", "error", sanitizeRedisError(err))
		return newMemoryCache(cfg), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisStartupTimeout)
	defer cancel()

	available := redisCache.Ping(ctx) == nil
	if !available {
		logger.Warn("redis unavailable; circuit breaker starts open")
	} else {
		logger.Info("redis cache connected")
	}

	memory := NewMemoryCache(cfg.DefaultTTL, cfg.DefaultLimit)
	return NewResilientCache(redisCache, memory, logger, available), nil
}

// sanitizeRedisError strips raw URLs and credentials from Redis errors.
func sanitizeRedisError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if idx := strings.Index(msg, "redis://"); idx != -1 {
		endIdx := strings.IndexAny(msg[idx:], " \t\r\n\"'")
		var urlPart string
		if endIdx == -1 {
			urlPart = msg[idx:]
		} else {
			urlPart = msg[idx : idx+endIdx]
		}
		msg = strings.ReplaceAll(msg, urlPart, "[redacted]")
	}
	return msg
}
