package retention

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

// groupTimeout bounds one group's cleanup so a slow delete cannot starve the others.
const groupTimeout = 30 * time.Second

type Store interface {
	DeleteEventsBefore(context.Context, time.Time, int) (int64, error)
	CountEventsBefore(context.Context, time.Time) (int64, error)
}

// MetricStore is optional so existing stores and tests can retain their small
// event-only contract. PostgreSQL implements it when OTLP metrics are enabled.
type MetricStore interface {
	DeleteMetricsBefore(context.Context, time.Time, int) (int64, error)
}

// HistoryStore deletes operational history. Deleting an incident also deletes
// its evidence, occurrences, status history and replay results, so no orphan
// rows remain; only resolved or ignored incidents are eligible.
type HistoryStore interface {
	DeleteIncidentsBefore(context.Context, time.Time, int) (int64, error)
	DeleteAuditBefore(context.Context, time.Time, int) (int64, error)
	DeleteDeliveriesBefore(context.Context, time.Time, int) (int64, error)
}

// Policy is how long each group of data is kept. Zero keeps a group forever.
type Policy struct {
	Telemetry  time.Duration
	Incidents  time.Duration
	Audit      time.Duration
	Deliveries time.Duration
}

// Enabled reports whether any group expires.
func (policy Policy) Enabled() bool {
	return policy.Telemetry > 0 || policy.Incidents > 0 || policy.Audit > 0 || policy.Deliveries > 0
}

type group struct {
	name   string
	maxAge time.Duration
	delete func(context.Context, time.Time, int) (int64, error)
}

// groups lists the cleanups the policy and the store support.
func (policy Policy) groups(store Store) []group {
	var groups []group
	if policy.Telemetry > 0 {
		groups = append(groups, group{"telemetry", policy.Telemetry, store.DeleteEventsBefore})
		if metricStore, ok := store.(MetricStore); ok {
			groups = append(groups, group{"metrics", policy.Telemetry, metricStore.DeleteMetricsBefore})
		}
	}
	if historyStore, ok := store.(HistoryStore); ok {
		for _, history := range []group{
			{"incidents", policy.Incidents, historyStore.DeleteIncidentsBefore},
			{"audit", policy.Audit, historyStore.DeleteAuditBefore},
			{"deliveries", policy.Deliveries, historyStore.DeleteDeliveriesBefore},
		} {
			if history.maxAge > 0 {
				groups = append(groups, history)
			}
		}
	}
	return groups
}

// Run cleans up expired data now and then every interval until ctx ends. A
// failing group is logged and counted but never stops the other groups.
func Run(ctx context.Context, logger *slog.Logger, store Store, selfMetrics *metrics.Registry, policy Policy, interval time.Duration, batchSize int) error {
	groups := policy.groups(store)
	prune := func() { pruneAll(ctx, logger, store, selfMetrics, groups, policy, batchSize) }

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

func pruneAll(ctx context.Context, logger *slog.Logger, store Store, selfMetrics *metrics.Registry, groups []group, policy Policy, batchSize int) {
	startedAt := time.Now()
	var failed bool
	var eventsDeleted int64
	for _, current := range groups {
		groupContext, cancel := context.WithTimeout(ctx, groupTimeout)
		deleted, pruneError := DeleteExpired(groupContext, time.Now().UTC().Add(-current.maxAge), batchSize, current.delete)
		cancel()
		selfMetrics.ObserveRetentionGroup(current.name, deleted)
		if current.name == "telemetry" {
			eventsDeleted = deleted
		}
		if pruneError != nil {
			failed = true
			logger.Error("retention cleanup failed", "group", current.name, "deleted", deleted, "error", pruneError)
		} else if deleted > 0 {
			logger.Info("retention cleanup completed", "group", current.name, "deleted", deleted)
		}
	}
	if policy.Telemetry > 0 {
		recordBacklog(ctx, logger, store, selfMetrics, policy.Telemetry)
	}
	selfMetrics.ObserveRetention(time.Since(startedAt), eventsDeleted, failed)
}

// recordBacklog reports the events still older than the cutoff after cleanup.
func recordBacklog(ctx context.Context, logger *slog.Logger, store Store, selfMetrics *metrics.Registry, maxAge time.Duration) {
	backlogContext, cancel := context.WithTimeout(ctx, groupTimeout)
	defer cancel()
	backlog, backlogError := store.CountEventsBefore(backlogContext, time.Now().UTC().Add(-maxAge))
	if backlogError != nil {
		logger.Warn("retention backlog metric failed", "error", backlogError)
		return
	}
	selfMetrics.SetRetentionBacklog(backlog)
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
