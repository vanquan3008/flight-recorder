// Package partition keeps the time partitions telemetry is written into ahead of the clock.
package partition

import (
	"context"
	"log/slog"
	"time"
)

// lookBackDays also covers records that arrive slightly late, just after midnight UTC.
const lookBackDays = 1

type Store interface {
	EnsurePartitions(ctx context.Context, from time.Time, days int) error
}

// Ensure creates any missing partition from yesterday through windowDays ahead.
func Ensure(ctx context.Context, store Store, windowDays int) error {
	return ensureAt(ctx, store, windowDays, time.Now())
}

func ensureAt(ctx context.Context, store Store, windowDays int, now time.Time) error {
	return store.EnsurePartitions(ctx, now.UTC().AddDate(0, 0, -lookBackDays), windowDays)
}

// Run repeats Ensure every interval until ctx ends. A failed pass is logged and retried
// on the next tick, so a short database outage never stops the process.
func Run(ctx context.Context, logger *slog.Logger, store Store, windowDays int, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := Ensure(ctx, store, windowDays); err != nil {
				logger.ErrorContext(ctx, "partition maintenance failed", "error", err)
			}
		}
	}
}
