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

// metadataMaintenance prunes incidents, their envelopes, and management history.
type metadataMaintenance interface {
	ListRetentionProjects(context.Context) ([]string, error)
	DropExpiredTelemetryPartitions(context.Context, time.Time, time.Duration) (events int64, err error)
	PruneProjectTelemetry(context.Context, string, time.Time, time.Duration, int) (events int64, err error)
	CountExpiredProjectEvents(context.Context, time.Time, time.Duration) (int64, error)
	DeleteIncidentsBefore(context.Context, time.Time, int) (int64, error)
	DeleteAuditBefore(context.Context, time.Time, int) (int64, error)
	DeleteDeliveriesBefore(context.Context, time.Time, int) (int64, error)
}

type telemetryMaintenance interface {
	DropExpiredPartitions(context.Context, time.Time, time.Duration) error
	PruneProject(context.Context, string, time.Time, time.Duration) error
}

// Policy is how long each group is kept. Telemetry is the default for projects without an override.
type Policy struct {
	Telemetry  time.Duration
	Incidents  time.Duration
	Audit      time.Duration
	Deliveries time.Duration
}

type group struct {
	name   string
	maxAge time.Duration
	delete func(context.Context, time.Time, int) (int64, error)
}

// Run cleans up expired data now and then every interval until ctx ends. A
// failing group is logged and counted but never stops the other groups.
func Run(ctx context.Context, logger *slog.Logger, metadata metadataMaintenance, telemetry telemetryMaintenance, selfMetrics *metrics.Registry, policy Policy, interval time.Duration, batchSize int, now func() time.Time) error {
	prune := func() { pruneAll(ctx, logger, metadata, telemetry, selfMetrics, policy, batchSize, now().UTC()) }

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

func pruneAll(ctx context.Context, logger *slog.Logger, metadata metadataMaintenance, telemetry telemetryMaintenance, selfMetrics *metrics.Registry, policy Policy, batchSize int, now time.Time) {
	startedAt := time.Now()
	var failed bool
	for _, current := range []group{
		{"incidents", policy.Incidents, metadata.DeleteIncidentsBefore},
		{"audit", policy.Audit, metadata.DeleteAuditBefore},
		{"deliveries", policy.Deliveries, metadata.DeleteDeliveriesBefore},
	} {
		if current.maxAge <= 0 {
			continue
		}
		groupContext, cancel := context.WithTimeout(ctx, groupTimeout)
		deleted, pruneError := DeleteExpired(groupContext, now.Add(-current.maxAge), batchSize, current.delete)
		cancel()
		selfMetrics.ObserveRetentionGroup(current.name, deleted)
		if pruneError != nil {
			failed = true
			logger.ErrorContext(ctx, "retention cleanup failed", "group", current.name, "deleted", deleted, "error", pruneError)
		} else if deleted > 0 {
			logger.InfoContext(ctx, "retention cleanup completed", "group", current.name, "deleted", deleted)
		}
	}
	events, telemetryError := pruneTelemetry(ctx, logger, metadata, telemetry, now, policy.Telemetry, batchSize)
	selfMetrics.ObserveRetentionGroup("telemetry", events)
	if telemetryError != nil && ctx.Err() == nil {
		logger.ErrorContext(ctx, "telemetry retention sweep incomplete", "error", telemetryError)
	}
	recordBacklog(ctx, logger, metadata, selfMetrics, now, policy.Telemetry)
	selfMetrics.ObserveRetention(time.Since(startedAt), events, failed || telemetryError != nil)
}

// pruneTelemetry expires incident envelopes in PostgreSQL and signals in
// ClickHouse: whole partitions first, then each project's shorter policy.
func pruneTelemetry(ctx context.Context, logger *slog.Logger, metadata metadataMaintenance, telemetry telemetryMaintenance, now time.Time, fallback time.Duration, batchSize int) (int64, error) {
	if batchSize <= 0 {
		return 0, errors.New("retention batch size must be positive")
	}
	work, cancel := context.WithTimeout(ctx, groupTimeout)
	events, dropError := metadata.DropExpiredTelemetryPartitions(work, now, fallback)
	cancel()
	work, cancel = context.WithTimeout(ctx, groupTimeout)
	dropError = errors.Join(dropError, telemetry.DropExpiredPartitions(work, now, fallback))
	cancel()
	work, cancel = context.WithTimeout(ctx, groupTimeout)
	projects, listError := metadata.ListRetentionProjects(work)
	cancel()
	if listError != nil {
		return events, errors.Join(dropError, listError)
	}
	var projectFailed bool
	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return events, err
		}
		work, cancel = context.WithTimeout(ctx, groupTimeout)
		removed, pgError := metadata.PruneProjectTelemetry(work, project, now, fallback, batchSize)
		cancel()
		events += removed
		work, cancel = context.WithTimeout(ctx, groupTimeout)
		telemetryError := telemetry.PruneProject(work, project, now, fallback)
		cancel()
		if err := errors.Join(pgError, telemetryError); err != nil {
			projectFailed = true
			logger.ErrorContext(ctx, "project retention cleanup failed", "project_id", project, "error", err)
		}
	}
	if projectFailed {
		return events, errors.Join(dropError, errors.New("one or more projects could not be pruned"))
	}
	return events, dropError
}

// recordBacklog reports the events still older than the cutoff after cleanup.
func recordBacklog(ctx context.Context, logger *slog.Logger, store metadataMaintenance, selfMetrics *metrics.Registry, now time.Time, maxAge time.Duration) {
	backlogContext, cancel := context.WithTimeout(ctx, groupTimeout)
	defer cancel()
	backlog, backlogError := store.CountExpiredProjectEvents(backlogContext, now, maxAge)
	if backlogError != nil {
		logger.WarnContext(ctx, "retention backlog metric failed", "error", backlogError)
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
