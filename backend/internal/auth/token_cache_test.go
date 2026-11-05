package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type mockTokenSource struct {
	calls  atomic.Int64
	tokens map[string]IngestToken
	err    error
}

func (m *mockTokenSource) GetIngestToken(_ context.Context, id string) (IngestToken, error) {
	m.calls.Add(1)
	if m.err != nil {
		return IngestToken{}, m.err
	}
	tok, found := m.tokens[id]
	if !found {
		return IngestToken{}, ErrIngestTokenNotFound
	}
	return tok, nil
}

func TestCachedTokenSourceHitsAndMisses(t *testing.T) {
	mock := &mockTokenSource{
		tokens: map[string]IngestToken{
			"tok-1": {ID: "tok-1", ProjectID: "proj-1"},
		},
	}
	now := time.Now()
	cache := NewCachedTokenSource(mock, 10*time.Second, 4096)
	cache.now = func() time.Time { return now }

	ctx := context.Background()
	// First lookup: queries source.
	tok, err := cache.GetIngestToken(ctx, "tok-1")
	if err != nil || tok.ProjectID != "proj-1" {
		t.Fatalf("unexpected token result: %+v, %v", tok, err)
	}
	if calls := mock.calls.Load(); calls != 1 {
		t.Fatalf("expected 1 call to source, got %d", calls)
	}

	// Second lookup within TTL: served from cache.
	tok2, err2 := cache.GetIngestToken(ctx, "tok-1")
	if err2 != nil || tok2.ProjectID != "proj-1" {
		t.Fatalf("unexpected second token result: %+v, %v", tok2, err2)
	}
	if calls := mock.calls.Load(); calls != 1 {
		t.Fatalf("expected cache hit (still 1 call), got %d", calls)
	}

	// Lookup after TTL: queries source again.
	cache.now = func() time.Time { return now.Add(11 * time.Second) }
	tok3, err3 := cache.GetIngestToken(ctx, "tok-1")
	if err3 != nil || tok3.ProjectID != "proj-1" {
		t.Fatalf("unexpected third token result: %+v, %v", tok3, err3)
	}
	if calls := mock.calls.Load(); calls != 2 {
		t.Fatalf("expected 2 calls after TTL expiration, got %d", calls)
	}
}

func TestCachedTokenSourceInvalidate(t *testing.T) {
	mock := &mockTokenSource{
		tokens: map[string]IngestToken{
			"tok-1": {ID: "tok-1", ProjectID: "proj-1"},
		},
	}
	now := time.Now()
	cache := NewCachedTokenSource(mock, time.Hour, 4096)
	cache.now = func() time.Time { return now }

	ctx := context.Background()
	_, _ = cache.GetIngestToken(ctx, "tok-1")
	if mock.calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.calls.Load())
	}

	// Invalidate token
	cache.Invalidate("tok-1")

	// Next call must hit source
	_, _ = cache.GetIngestToken(ctx, "tok-1")
	if mock.calls.Load() != 2 {
		t.Fatalf("expected 2 calls after invalidation, got %d", mock.calls.Load())
	}
}

func TestCachedTokenSourceHonorsEntryLimit(t *testing.T) {
	mock := &mockTokenSource{
		tokens: map[string]IngestToken{
			"tok-1": {ID: "tok-1", ProjectID: "proj-1"},
			"tok-2": {ID: "tok-2", ProjectID: "proj-2"},
		},
	}
	cache := NewCachedTokenSource(mock, time.Hour, 1)
	ctx := context.Background()

	for _, tokenID := range []string{"tok-1", "tok-2", "tok-1"} {
		if _, err := cache.GetIngestToken(ctx, tokenID); err != nil {
			t.Fatalf("get token %q: %v", tokenID, err)
		}
	}
	if calls := mock.calls.Load(); calls != 3 {
		t.Fatalf("entry limit was not enforced; source calls = %d, want 3", calls)
	}
}

func TestCachedTokenSourceDoesNotCacheTransientErrors(t *testing.T) {
	transientErr := errors.New("database timeout")
	mock := &mockTokenSource{err: transientErr}
	cache := NewCachedTokenSource(mock, time.Hour, 4096)

	ctx := context.Background()
	_, err := cache.GetIngestToken(ctx, "tok-1")
	if !errors.Is(err, transientErr) {
		t.Fatalf("expected transientErr, got %v", err)
	}

	// Next call should re-query source, not use a cached error.
	_, _ = cache.GetIngestToken(ctx, "tok-1")
	if mock.calls.Load() != 2 {
		t.Fatalf("expected transient error not to be cached, got %d calls", mock.calls.Load())
	}
}

type testCacheObserver struct {
	hits   atomic.Int64
	misses atomic.Int64
}

func (o *testCacheObserver) ObserveAuthCache(kind, result string) {
	if kind == "token" {
		if result == "hit" {
			o.hits.Add(1)
		} else if result == "miss" {
			o.misses.Add(1)
		}
	}
}

func TestCachedTokenSourceObserver(t *testing.T) {
	mock := &mockTokenSource{
		tokens: map[string]IngestToken{
			"tok-1": {ID: "tok-1", ProjectID: "proj-1"},
		},
	}
	obs := &testCacheObserver{}
	cache := NewCachedTokenSource(mock, time.Hour, 4096).WithObserver(obs)

	ctx := context.Background()
	_, _ = cache.GetIngestToken(ctx, "tok-1") // miss
	_, _ = cache.GetIngestToken(ctx, "tok-1") // hit

	if obs.misses.Load() != 1 {
		t.Fatalf("expected 1 miss, got %d", obs.misses.Load())
	}
	if obs.hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", obs.hits.Load())
	}
}

type mockByteCache struct {
	data map[string][]byte
}

func (m *mockByteCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockByteCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *mockByteCache) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func TestCachedTokenSourceWithByteCache(t *testing.T) {
	mock := &mockTokenSource{
		tokens: map[string]IngestToken{
			"tok-1": {ID: "tok-1", ProjectID: "proj-1"},
		},
	}
	byteCache := &mockByteCache{data: make(map[string][]byte)}
	cache := NewCachedTokenSource(mock, time.Hour, 4096).WithByteCache(byteCache)

	ctx := context.Background()
	// First lookup populates both in-memory and byteCache
	tok, err := cache.GetIngestToken(ctx, "tok-1")
	if err != nil || tok.ProjectID != "proj-1" {
		t.Fatalf("unexpected token: %+v, %v", tok, err)
	}
	if len(byteCache.data) == 0 {
		t.Fatalf("expected byteCache to be populated")
	}

	// Create a new in-memory cache instance sharing the same byteCache (simulating another replica)
	replicaCache := NewCachedTokenSource(mock, time.Hour, 4096).WithByteCache(byteCache)
	tokReplica, err := replicaCache.GetIngestToken(ctx, "tok-1")
	if err != nil || tokReplica.ProjectID != "proj-1" {
		t.Fatalf("unexpected replica token: %+v, %v", tokReplica, err)
	}
	// Source was only called once by the first instance!
	if mock.calls.Load() != 1 {
		t.Fatalf("expected 1 call to database source, got %d", mock.calls.Load())
	}

	// Invalidation removes from byteCache too
	cache.Invalidate("tok-1")
	if _, ok := byteCache.data["ingest_token:tok-1"]; ok {
		t.Fatalf("expected key to be deleted from byteCache")
	}
}
