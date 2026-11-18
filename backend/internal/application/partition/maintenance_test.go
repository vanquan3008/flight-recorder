package partition

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type storeFunc func(context.Context, time.Time, int) error

func (store storeFunc) EnsurePartitions(ctx context.Context, from time.Time, days int) error {
	return store(ctx, from, days)
}

func TestEnsureStartsYesterdayInUTC(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 30, 0, 0, time.FixedZone("ICT", 7*60*60))
	var gotFrom time.Time
	var gotDays int
	err := ensureAt(context.Background(), storeFunc(func(_ context.Context, from time.Time, days int) error {
		gotFrom, gotDays = from, days
		return nil
	}), 10, now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 2, 27, 17, 30, 0, 0, time.UTC)
	if !gotFrom.Equal(want) || gotFrom.Location() != time.UTC || gotDays != 10 {
		t.Fatalf("EnsurePartitions(%v, %d), want (%v, 10)", gotFrom, gotDays, want)
	}
}

func TestRunRetriesAfterFailureAndStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	store := storeFunc(func(context.Context, time.Time, int) error {
		if calls.Add(1) >= 2 {
			cancel()
		}
		return errors.New("database unavailable")
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, logger, store, 10, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ctx.Err(), context.Canceled) || calls.Load() < 2 {
		t.Fatalf("maintenance stopped after %d passes: %v", calls.Load(), ctx.Err())
	}
}
