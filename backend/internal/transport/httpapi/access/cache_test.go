package access

import (
	"sync"
	"testing"
	"time"
)

func TestTTLCacheExpiresEntriesAfterTheirTTL(t *testing.T) {
	cache := newTTLCache[string](time.Minute, 10)
	loadedAt := time.Unix(1_700_000_000, 0)
	cache.put("owner", loadedAt, "project-1:user-1")

	if role, found := cache.get("project-1:user-1", loadedAt.Add(59*time.Second)); !found || role != "owner" {
		t.Fatalf("get before the TTL = (%q, %v), want (owner, true)", role, found)
	}
	if _, found := cache.get("project-1:user-1", loadedAt.Add(time.Minute)); found {
		t.Fatal("an entry was still served once its TTL had passed")
	}
}

func TestTTLCacheStoresOneValueUnderEveryKeyAndForgetsThem(t *testing.T) {
	cache := newTTLCache[string](time.Minute, 10)
	now := time.Now()
	cache.put("checkout", now, "checkout-slug", "project-id", "")

	for _, key := range []string{"checkout-slug", "project-id"} {
		if value, found := cache.get(key, now); !found || value != "checkout" {
			t.Fatalf("get(%q) = (%q, %v), want (checkout, true)", key, value, found)
		}
	}
	if _, found := cache.get("", now); found {
		t.Fatal("an empty key was stored")
	}

	cache.forget("project-id")
	if _, found := cache.get("project-id", now); found {
		t.Fatal("a forgotten key was still served")
	}
}

func TestTTLCacheStartsOverWhenFull(t *testing.T) {
	cache := newTTLCache[int](time.Minute, 2)
	now := time.Now()
	cache.put(1, now, "first")
	cache.put(2, now, "second")
	cache.put(3, now, "third")

	if _, found := cache.get("first", now); found {
		t.Fatal("a full cache kept its old entries")
	}
	if value, found := cache.get("third", now); !found || value != 3 {
		t.Fatalf("get(third) = (%d, %v), want (3, true)", value, found)
	}
}

func TestNilTTLCacheStoresNothing(t *testing.T) {
	var missing *ttlCache[int]
	missing.put(1, time.Now(), "key")
	missing.forget("key")
	if _, found := missing.get("key", time.Now()); found {
		t.Fatal("a nil cache served a value")
	}
}

func TestTTLCacheIsSafeForConcurrentUse(t *testing.T) {
	cache := newTTLCache[int](time.Minute, 64)
	now := time.Now()
	var group sync.WaitGroup
	for worker := range 8 {
		group.Go(func() {
			for index := range 200 {
				key := roleCacheKey("project", string(rune('a'+worker)))
				cache.put(index, now, key)
				cache.get(key, now)
				if index%10 == 0 {
					cache.forget(key)
				}
			}
		})
	}
	group.Wait()
}
