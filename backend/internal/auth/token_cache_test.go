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
	cache := NewCachedTokenSource(mock, 10*time.Second)
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
	cache := NewCachedTokenSource(mock, time.Hour)
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

func TestCachedTokenSourceDoesNotCacheTransientErrors(t *testing.T) {
	transientErr := errors.New("database timeout")
	mock := &mockTokenSource{err: transientErr}
	cache := NewCachedTokenSource(mock, time.Hour)

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
	cache := NewCachedTokenSource(mock, time.Hour).WithObserver(obs)

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

