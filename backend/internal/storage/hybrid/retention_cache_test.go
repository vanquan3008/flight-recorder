package hybrid

import (
	"context"
	"errors"
	"testing"
	"time"
)

type retentionLoader struct {
	days   *int32
	err    error
	calls  int
	during func()
}

func (loader *retentionLoader) load(context.Context, string) (*int32, error) {
	loader.calls++
	days, err := loader.days, loader.err
	if loader.during != nil {
		loader.during()
	}
	return days, err
}

func TestRetentionCacheServesRepeatedReadsUntilTheTTLEnds(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	cache := newRetentionCache(30*time.Second, 16)
	cache.now = func() time.Time { return now }
	seven := int32(7)
	loader := &retentionLoader{days: &seven}
	for range 3 {
		days, err := cache.get(context.Background(), "project", loader.load)
		if err != nil || days == nil || *days != 7 {
			t.Fatalf("days = %v, err = %v", days, err)
		}
	}
	if loader.calls != 1 {
		t.Fatalf("PostgreSQL was read %d times within the TTL, want once", loader.calls)
	}
	now = now.Add(31 * time.Second)
	if _, err := cache.get(context.Background(), "project", loader.load); err != nil || loader.calls != 2 {
		t.Fatalf("an expired entry was not reloaded: calls=%d err=%v", loader.calls, err)
	}
}

func TestRetentionCacheForgetsAnUpdatedPolicyAtOnce(t *testing.T) {
	cache := newRetentionCache(time.Minute, 16)
	seven, one := int32(7), int32(1)
	loader := &retentionLoader{days: &seven}
	if _, err := cache.get(context.Background(), "project", loader.load); err != nil {
		t.Fatal(err)
	}
	loader.days = &one
	cache.forget("project")
	if days, err := cache.get(context.Background(), "project", loader.load); err != nil || days == nil || *days != 1 {
		t.Fatalf("a shortened policy was not applied after the update: %v, %v", days, err)
	}
}

func TestRetentionCacheDoesNotKeepAValueLoadedDuringAnUpdate(t *testing.T) {
	cache := newRetentionCache(time.Minute, 16)
	seven, one := int32(7), int32(1)
	loader := &retentionLoader{days: &seven}
	loader.during = func() {
		loader.days = &one
		cache.forget("project")
		loader.during = nil
	}
	if days, _ := cache.get(context.Background(), "project", loader.load); days == nil || *days != 7 {
		t.Fatalf("the read that raced the update returned %v", days)
	}
	if days, _ := cache.get(context.Background(), "project", loader.load); days == nil || *days != 1 || loader.calls != 2 {
		t.Fatalf("a value read before the update outlived it: days=%v calls=%d", days, loader.calls)
	}
}

func TestRetentionCacheNeverCachesErrorsAndStaysBounded(t *testing.T) {
	cache := newRetentionCache(time.Minute, 2)
	loader := &retentionLoader{err: errors.New("database unavailable")}
	for range 2 {
		if _, err := cache.get(context.Background(), "project", loader.load); err == nil {
			t.Fatal("a load error was hidden")
		}
	}
	if loader.calls != 2 {
		t.Fatalf("a failed load was cached: calls=%d", loader.calls)
	}
	loader.err = nil
	for _, project := range []string{"a", "b", "c"} {
		if _, err := cache.get(context.Background(), project, loader.load); err != nil {
			t.Fatal(err)
		}
	}
	if len(cache.entries) > 2 {
		t.Fatalf("cache grew to %d entries, limit 2", len(cache.entries))
	}
}

func TestNilRetentionCacheReadsThrough(t *testing.T) {
	var cache *retentionCache
	loader := &retentionLoader{}
	for range 2 {
		if _, err := cache.get(context.Background(), "project", loader.load); err != nil {
			t.Fatal(err)
		}
	}
	cache.forget("project")
	if loader.calls != 2 {
		t.Fatalf("calls = %d, want every read to reach the loader", loader.calls)
	}
}
