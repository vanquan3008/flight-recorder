package main

import (
	"context"
	"log/slog"
	"time"
)

const (
	partitionLookBackDays = 1
	partitionWindowDays   = 10
	partitionInterval     = time.Hour
)

type partitionStore interface {
	EnsurePartitions(context.Context, time.Time, int) error
}

func ensureTelemetryPartitions(ctx context.Context, store partitionStore) error {
	return store.EnsurePartitions(ctx, time.Now().UTC().AddDate(0, 0, -partitionLookBackDays), partitionWindowDays)
}

func runPartitionMaintenance(ctx context.Context, logger *slog.Logger, store partitionStore) error {
	ticker := time.NewTicker(partitionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := ensureTelemetryPartitions(ctx, store); err != nil {
				logger.Error("partition maintenance failed", "error", err)
			}
		}
	}
}
