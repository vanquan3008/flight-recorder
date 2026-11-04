package cache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vanquan3008/tracepilot/internal/config"
)

// RedisCache provides a Redis-backed distributed cache store.
type RedisCache struct {
	client     *redis.Client
	defaultTTL time.Duration
}

// NewRedisCache constructs a Redis client using configuration URL or Address.
func NewRedisCache(cfg config.CacheConfig) (*RedisCache, error) {
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

	client := redis.NewClient(opts)
	defaultTTL := cfg.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 60 * time.Second
	}

	return &RedisCache{
		client:     client,
		defaultTTL: defaultTTL,
	}, nil
}

func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = r.defaultTTL
	}
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}
