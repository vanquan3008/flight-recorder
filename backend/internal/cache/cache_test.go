package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
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
