package cache

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestRedisCache_NilSafety(t *testing.T) {
	ctx := context.Background()
	var uninitialized *RedisCache

	if err := uninitialized.Ping(ctx); err == nil {
		t.Fatalf("expected error from uninitialized Ping")
	}

	val, ok, err := uninitialized.Get(ctx, "key")
	if err == nil || ok || val != nil {
		t.Fatalf("expected error from uninitialized Get")
	}

	if err := uninitialized.Set(ctx, "key", []byte("v"), time.Minute); err == nil {
		t.Fatalf("expected error from uninitialized Set")
	}

	if err := uninitialized.Delete(ctx, "key"); err == nil {
		t.Fatalf("expected error from uninitialized Delete")
	}

	if err := uninitialized.Close(); err != nil {
		t.Fatalf("expected nil error from uninitialized Close, got: %v", err)
	}
}

func TestResilientCache_FallbackWhenRedisDown(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache(time.Minute, 100)

	// Initialized with Redis marked as unhealthy (circuit open)
	rc := NewResilientCache(nil, mem, nil, false)

	if rc.State() != StateOpen {
		t.Fatalf("expected initial state StateOpen, got %v", rc.State())
	}

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

func TestResilientCache_CircuitBreakerStateTransitions(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache(time.Minute, 100)
	now := time.Now()

	dummyRedis := &RedisCache{}
	rc := NewResilientCache(dummyRedis, mem, nil, false)
	rc.now = func() time.Time { return now }
	rc.cooldown = 5 * time.Second

	// Initially Open
	if rc.State() != StateOpen {
		t.Fatalf("expected StateOpen, got %v", rc.State())
	}

	// Before cooldown: should still be Open
	if rc.shouldAttemptRedis() {
		t.Fatalf("expected Redis attempt to be rejected before cooldown")
	}

	// Advance time past cooldown
	now = now.Add(6 * time.Second)

	// Now should transition to HalfOpen
	if !rc.shouldAttemptRedis() {
		t.Fatalf("expected Redis attempt to be allowed after cooldown")
	}
	if rc.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %v", rc.State())
	}

	// If probe fails in HalfOpen, trips back to Open immediately
	rc.onFailure(ctx, "probe", "test", errors.New("connection refused"))
	if rc.State() != StateOpen {
		t.Fatalf("expected StateOpen after failed probe, got %v", rc.State())
	}

	// Advance time past cooldown again
	now = now.Add(6 * time.Second)
	_ = rc.shouldAttemptRedis()
	if rc.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %v", rc.State())
	}

	// Probe succeeds in HalfOpen: recovers to Closed!
	rc.onSuccess()
	if rc.State() != StateClosed {
		t.Fatalf("expected StateClosed after successful recovery, got %v", rc.State())
	}
}

func TestInvalidRedisConfigurationDoesNotLogCredentials(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	c, err := New(config.CacheConfig{
		Driver: config.CacheDriverRedis,
		URL:    "redis://audit:do-not-log-this@localhost:invalid",
	}, logger)

	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	logStr := output.String()
	if strings.Contains(logStr, "do-not-log-this") || strings.Contains(logStr, "redis://") {
		t.Fatalf("cache diagnostic leaked credentials or raw URL in logs: %s", logStr)
	}
}

func TestFactory_Drivers(t *testing.T) {
	ctx := context.Background()

	// 1. Noop driver
	noopCache, err := New(config.CacheConfig{Driver: config.CacheDriverNoop}, nil)
	if err != nil {
		t.Fatalf("failed to init noop: %v", err)
	}
	_ = noopCache.Set(ctx, "k", []byte("v"), time.Minute)
	val, ok, _ := noopCache.Get(ctx, "k")
	if ok || val != nil {
		t.Fatalf("expected miss on noop")
	}
	_ = noopCache.Close()

	// 2. Memory driver
	memCache, err := New(config.CacheConfig{
		Driver:       config.CacheDriverMemory,
		DefaultTTL:   time.Minute,
		DefaultLimit: 50,
	}, nil)
	if err != nil {
		t.Fatalf("failed to init memory: %v", err)
	}
	_ = memCache.Set(ctx, "hello", []byte("world"), time.Minute)
	val, ok, err = memCache.Get(ctx, "hello")
	if err != nil || !ok || string(val) != "world" {
		t.Fatalf("expected 'world', got %s", string(val))
	}
	_ = memCache.Close()
}
