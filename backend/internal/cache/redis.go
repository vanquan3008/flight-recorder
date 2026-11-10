package cache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vanquan3008/tracepilot/internal/config"
)

// RedisCache provides a Redis-backed distributed cache store with connection pooling and timeouts.
type RedisCache struct {
	client     *redis.Client
	defaultTTL time.Duration
	metrics    Metrics
}

// NewRedisCache constructs a Redis client using configuration URL or Address.
func NewRedisCache(cfg config.CacheConfig) (*RedisCache, error) {
	return NewRedisCacheWithMetrics(cfg, nil)
}

// NewRedisCacheWithMetrics constructs a Redis client with an optional metrics collector.
func NewRedisCacheWithMetrics(cfg config.CacheConfig, metrics Metrics) (*RedisCache, error) {
	var opts *redis.Options

	if strings.TrimSpace(cfg.URL) != "" {
		parsed, err := redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, err
		}
		opts = parsed
	} else if strings.TrimSpace(cfg.Address) != "" {
		opts = &redis.Options{
			Addr:     cfg.Address,
			Password: cfg.Password,
			DB:       cfg.DB,
		}
	} else {
		return nil, errors.New("redis cache: neither url nor address was provided")
	}

	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = 1 * time.Second
	opts.WriteTimeout = 1 * time.Second
	opts.PoolSize = 20
	opts.MinIdleConns = 5

	client := redis.NewClient(opts)
	defaultTTL := cfg.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 60 * time.Second
	}

	return &RedisCache{
		client:     client,
		defaultTTL: defaultTTL,
		metrics:    metrics,
	}, nil
}

// Ping checks whether the Redis server is reachable and responsive.
func (r *RedisCache) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("redis cache: client is not initialized")
	}
	return r.client.Ping(ctx).Err()
}

// Get retrieves the raw byte value stored at key.
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if r == nil || r.client == nil {
		return nil, false, errors.New("redis cache: client is not initialized")
	}

	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		if r.metrics != nil {
			r.metrics.RecordMiss("redis")
		}
		return nil, false, nil
	}
	if err != nil {
		if r.metrics != nil {
			r.metrics.RecordError("redis", "get")
		}
		return nil, false, err
	}

	if r.metrics != nil {
		r.metrics.RecordHit("redis")
	}
	return val, true, nil
}

// Set stores a value with the specified TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if r == nil || r.client == nil {
		return errors.New("redis cache: client is not initialized")
	}

	if ttl <= 0 {
		ttl = r.defaultTTL
	}
	err := r.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		if r.metrics != nil {
			r.metrics.RecordError("redis", "set")
		}
		return err
	}
	if r.metrics != nil {
		r.metrics.RecordSet("redis")
	}
	return nil
}

// Delete removes a key from Redis.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	if r == nil || r.client == nil {
		return errors.New("redis cache: client is not initialized")
	}

	err := r.client.Del(ctx, key).Err()
	if err != nil {
		if r.metrics != nil {
			r.metrics.RecordError("redis", "delete")
		}
		return err
	}
	if r.metrics != nil {
		r.metrics.RecordDelete("redis")
	}
	return nil
}

// Close gracefully closes the underlying Redis client connection pool.
func (r *RedisCache) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}
