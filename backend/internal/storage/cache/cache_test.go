package cache

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestMemoryCacheGetSetDelete(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 100)

	// Miss
	val, ok, err := c.Get(ctx, "nonexistent")
	if err != nil || ok || val != nil {
		t.Fatalf("expected miss, got val=%v, ok=%t, err=%v", val, ok, err)
	}

	// Set and Hit
	err = c.Set(ctx, "k1", []byte("v1"), time.Minute)
	if err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}

	val, ok, err = c.Get(ctx, "k1")
	if err != nil || !ok || string(val) != "v1" {
		t.Fatalf("expected hit with 'v1', got val=%s, ok=%t, err=%v", string(val), ok, err)
	}

	// Delete
	err = c.Delete(ctx, "k1")
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}

	val, ok, err = c.Get(ctx, "k1")
	if err != nil || ok {
		t.Fatalf("expected miss after delete, got ok=%t", ok)
	}
}

func TestMemoryCacheExpiry(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 100)
	now := time.Now()
	c.now = func() time.Time { return now }

	_ = c.Set(ctx, "expiring", []byte("temp"), 10*time.Millisecond)

	// Advance time past TTL
	c.now = func() time.Time { return now.Add(20 * time.Millisecond) }

	val, ok, err := c.Get(ctx, "expiring")
	if err != nil || ok || val != nil {
		t.Fatalf("expected expired entry to be missed, got ok=%t", ok)
	}
}

func TestResilientCacheFallbackWhenRedisDown(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache(time.Minute, 100)

	// Resilient cache initialized with Redis marked as unhealthy
	rc := NewResilientCache(nil, mem, nil, false)

	err := rc.Set(ctx, "fallback-key", []byte("fallback-val"), time.Minute)
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	val, ok, err := rc.Get(ctx, "fallback-key")
	if err != nil || !ok || string(val) != "fallback-val" {
		t.Fatalf("expected fallback hit, got val=%s, ok=%t, err=%v", string(val), ok, err)
	}

	err = rc.Delete(ctx, "fallback-key")
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}

	_, ok, _ = rc.Get(ctx, "fallback-key")
	if ok {
		t.Fatalf("expected miss after delete")
	}
}

func TestNewWithMemoryDriver(t *testing.T) {
	cfg := config.CacheConfig{
		Driver:       config.CacheDriverMemory,
		DefaultTTL:   30 * time.Second,
		DefaultLimit: 1000,
	}

	c, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to initialize memory cache: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "test", []byte("123"), time.Second)
	v, ok, _ := c.Get(ctx, "test")
	if !ok || string(v) != "123" {
		t.Fatalf("expected '123', got %s", string(v))
	}
}
