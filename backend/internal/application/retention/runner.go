package retention

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

type Store interface {
	DeleteEventsBefore(context.Context, time.Time, int) (int64, error)
	CountEventsBefore(context.Context, time.Time) (int64, error)
}

// MetricStore is optional so existing stores and tests can retain their small
// event-only contract. PostgreSQL implements it when OTLP metrics are enabled.
type MetricStore interface {
	DeleteMetricsBefore(context.Context, time.Time, int) (int64, error)
}

func Run(ctx context.Context, logger *slog.Logger, store Store, selfMetrics *metrics.Registry, maxAge, interval time.Duration, batchSize int) error {
	prune := func() {
		startedAt := time.Now()
		pruneContext, cancelPrune := context.WithTimeout(ctx, 30*time.Second)
		before := time.Now().UTC().Add(-maxAge)
		deleted, pruneError := DeleteExpired(pruneContext, before, batchSize, store.DeleteEventsBefore)
		if metricStore, ok := store.(MetricStore); ok {
			metricDeleted, metricError := DeleteExpired(pruneContext, before, batchSize, metricStore.DeleteMetricsBefore)
			if metricError != nil {
				pruneError = errors.Join(pruneError, metricError)
			} else if metricDeleted > 0 {
				logger.Info("metric retention cleanup completed", "deleted_points", metricDeleted)
			}
		}
		if pruneError == nil {
			if backlog, backlogError := store.CountEventsBefore(pruneContext, before); backlogError != nil {
				logger.Warn("retention backlog metric failed", "error", backlogError)
			} else {
				selfMetrics.SetRetentionBacklog(backlog)
			}
		}
		cancelPrune()
		selfMetrics.ObserveRetention(time.Since(startedAt), deleted, pruneError != nil)
		if pruneError != nil {
			logger.Error("retention cleanup failed", "error", pruneError)
			return
		}
		if deleted > 0 {
			logger.Info("retention cleanup completed", "deleted_events", deleted)
		}
	}

	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			prune()
		}
	}
}

func DeleteExpired(ctx context.Context, before time.Time, batchSize int, deleteBatch func(context.Context, time.Time, int) (int64, error)) (int64, error) {
	if batchSize <= 0 {
		return 0, errors.New("retention batch size must be positive")
	}
	var deleted int64
	for {
		if ctx.Err() != nil {
			return deleted, ctx.Err()
		}
		batchDeleted, deleteError := deleteBatch(ctx, before, batchSize)
		deleted += batchDeleted
		if deleteError != nil {
			return deleted, deleteError
		}
		if batchDeleted < int64(batchSize) {
			return deleted, nil
		}
	}
}
