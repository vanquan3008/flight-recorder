package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestNoopCache(t *testing.T) {
	ctx := context.Background()
	c := NewNoopCache()

	val, ok, err := c.Get(ctx, "any-key")
	if err != nil || ok || val != nil {
		t.Fatalf("expected miss on noop cache, got val=%v, ok=%t, err=%v", val, ok, err)
	}

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestMemoryCache_GetSetDelete(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 100)

	// Miss on non-existent key
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

	// Delete and verify miss
	err = c.Delete(ctx, "k1")
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}

	val, ok, err = c.Get(ctx, "k1")
	if err != nil || ok {
		t.Fatalf("expected miss after delete, got ok=%t", ok)
	}
}

func TestMemoryCache_DeepCopy(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 100)

	original := []byte("hello")
	_ = c.Set(ctx, "k", original, time.Minute)

	// Mutate caller's original slice: should not alter cached value
	original[0] = 'X'

	val, ok, _ := c.Get(ctx, "k")
	if !ok || string(val) != "hello" {
		t.Fatalf("cache was corrupted by external slice mutation: %s", string(val))
	}

	// Mutate returned slice: should not alter cached value
	val[0] = 'Y'

	val2, _, _ := c.Get(ctx, "k")
	if string(val2) != "hello" {
		t.Fatalf("cache was corrupted by returned slice mutation: %s", string(val2))
	}
}

func TestMemoryCache_Expiry(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 100)
	now := time.Now()
	c.now = func() time.Time { return now }

	_ = c.Set(ctx, "expiring", []byte("temp"), 100*time.Millisecond)

	// Before expiration: hit
	val, ok, err := c.Get(ctx, "expiring")
	if err != nil || !ok || string(val) != "temp" {
		t.Fatalf("expected hit before expiration, got ok=%t", ok)
	}

	// Advance time past TTL: miss
	c.now = func() time.Time { return now.Add(200 * time.Millisecond) }

	val, ok, err = c.Get(ctx, "expiring")
	if err != nil || ok || val != nil {
		t.Fatalf("expected expired entry to be missed, got ok=%t", ok)
	}
}

func TestMemoryCache_EvictionAtCapacity(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 3)
	now := time.Now()
	c.now = func() time.Time { return now }

	_ = c.Set(ctx, "k1", []byte("v1"), time.Minute)
	now = now.Add(time.Second)
	_ = c.Set(ctx, "k2", []byte("v2"), time.Minute)
	now = now.Add(time.Second)
	_ = c.Set(ctx, "k3", []byte("v3"), time.Minute)

	if c.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", c.Len())
	}

	// Add 4th item, exceeding limit of 3 -> oldest (k1) should be evicted
	now = now.Add(time.Second)
	_ = c.Set(ctx, "k4", []byte("v4"), time.Minute)

	if c.Len() > 3 {
		t.Fatalf("cache capacity exceeded: len=%d", c.Len())
	}

	_, ok1, _ := c.Get(ctx, "k1")
	_, ok4, _ := c.Get(ctx, "k4")
	if ok1 {
		t.Fatalf("expected k1 to be evicted")
	}
	if !ok4 {
		t.Fatalf("expected k4 to be present")
	}
}

func TestMemoryCache_Concurrency(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(time.Minute, 500)

	var wg sync.WaitGroup
	workers := 40
	opsPerWorker := 100

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				key := fmt.Sprintf("key-%d-%d", workerID, j%10)
				val := []byte(fmt.Sprintf("val-%d", j))

				_ = c.Set(ctx, key, val, 10*time.Second)
				_, _, _ = c.Get(ctx, key)
				if j%5 == 0 {
					_ = c.Delete(ctx, key)
				}
			}
		}()
	}

	wg.Wait()
}

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
	metrics := NewStatsCollector()

	// Initialized with Redis marked as unhealthy (circuit open)
	rc := NewResilientCache(nil, mem, nil, metrics, false)

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

	snapshot := metrics.Snapshot()
	if snapshot.FallbacksTotal == 0 {
		t.Fatalf("expected fallback metrics to be recorded, got 0")
	}
}

func TestResilientCache_CircuitBreakerStateTransitions(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache(time.Minute, 100)
	now := time.Now()

	dummyRedis := &RedisCache{}
	rc := NewResilientCache(dummyRedis, mem, nil, nil, false)
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

func TestStatsCollector_Counters(t *testing.T) {
	sc := NewStatsCollector()

	sc.RecordHit("memory")
	sc.RecordHit("redis")
	sc.RecordMiss("redis")
	sc.RecordSet("redis")
	sc.RecordDelete("memory")
	sc.RecordEviction("memory", "expired")
	sc.RecordFallback("get")
	sc.RecordCircuitBreakerState("open")

	snap := sc.Snapshot()
	if snap.HitsTotal != 2 {
		t.Fatalf("expected 2 total hits, got %d", snap.HitsTotal)
	}
	if snap.MissesTotal != 1 {
		t.Fatalf("expected 1 total miss, got %d", snap.MissesTotal)
	}
	if snap.SetsTotal != 1 {
		t.Fatalf("expected 1 total set, got %d", snap.SetsTotal)
	}
	if snap.FallbacksTotal != 1 {
		t.Fatalf("expected 1 fallback, got %d", snap.FallbacksTotal)
	}
	if snap.LastCircuitState != "open" {
		t.Fatalf("expected last circuit state 'open', got %s", snap.LastCircuitState)
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
