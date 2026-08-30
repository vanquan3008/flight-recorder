package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPruneExpiredEventsDrainsBacklog(t *testing.T) {
	calls := 0
	deleted, pruneError := pruneExpiredEvents(context.Background(), time.Now(), 2, func(context.Context, time.Time, int) (int64, error) {
		calls++
		if calls < 3 {
			return 2, nil
		}
		return 1, nil
	})
	if pruneError != nil || deleted != 5 || calls != 3 {
		t.Fatalf("deleted=%d calls=%d error=%v", deleted, calls, pruneError)
	}
}

func TestPruneExpiredEventsStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	deleted, pruneError := pruneExpiredEvents(ctx, time.Now(), 1, func(context.Context, time.Time, int) (int64, error) {
		calls++
		cancel()
		return 1, nil
	})
	if !errors.Is(pruneError, context.Canceled) || deleted != 1 || calls != 1 {
		t.Fatalf("deleted=%d calls=%d error=%v", deleted, calls, pruneError)
	}
}

func TestPruneExpiredEventsReturnsDeleteError(t *testing.T) {
	wantError := errors.New("database unavailable")
	deleted, pruneError := pruneExpiredEvents(context.Background(), time.Now(), 10, func(context.Context, time.Time, int) (int64, error) {
		return 3, wantError
	})
	if !errors.Is(pruneError, wantError) || deleted != 3 {
		t.Fatalf("deleted=%d error=%v", deleted, pruneError)
	}
}
