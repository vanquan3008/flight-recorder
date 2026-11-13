package main

import (
	"context"
	"log/slog"
	"time"
)

const partitionLookBackDays = 1

type partitionStore interface {
	EnsurePartitions(context.Context, time.Time, int) error
}

func ensureTelemetryPartitions(ctx context.Context, store partitionStore, windowDays int) error {
	return store.EnsurePartitions(ctx, time.Now().UTC().AddDate(0, 0, -partitionLookBackDays), windowDays)
}

func runPartitionMaintenance(ctx context.Context, logger *slog.Logger, store partitionStore, windowDays int, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := ensureTelemetryPartitions(ctx, store, windowDays); err != nil {
				logger.ErrorContext(ctx, "partition maintenance failed", "error", err)
			}
		}
	}
}
