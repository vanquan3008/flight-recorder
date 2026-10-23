package main

import (
	"context"
	"log/slog"
	"time"
)

const (
	// Partitions cover yesterday (late data) through a week ahead, so a
	// maintenance outage of several days still leaves room to ingest.
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

// runPartitionMaintenance keeps the daily partitions ahead of the clock. A
// failure is logged and retried on the next tick: rows that miss a partition
// are accepted into the default partition, so ingestion keeps working.
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
