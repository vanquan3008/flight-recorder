package maintenance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// DropExpiredTelemetryPartitions drops only days expired under every project's policy.
// The catalog lock keeps the set of policies stable until the drop commits.
func (store *PartitionStore) DropExpiredTelemetryPartitions(ctx context.Context, now time.Time, fallback time.Duration) (int64, int64, error) {
	var events, points int64
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		queries := store.queries.WithTx(tx)
		if err := queries.AcquireTransactionLock(ctx, partitionLockKey); err != nil {
			return err
		}
		if err := queries.LockProjectRetentionCatalog(ctx); err != nil {
			return err
		}
		projects, err := queries.ListProjectRetention(ctx)
		if err != nil {
			return err
		}
		var longest time.Duration
		for _, project := range projects {
			age := telemetry.RetentionAge(project.TelemetryRetentionDays, fallback)
			if age == 0 {
				return nil
			}
			longest = max(longest, age)
		}
		if longest == 0 {
			return nil
		}
		before := now.Add(-longest)
		transaction := &PartitionStore{pool: tx, queries: queries}
		events, err = transaction.dropExpiredPartitions(ctx, eventPartitionedTables, before, transaction.countRows)
		if err != nil {
			return err
		}
		points, err = transaction.dropExpiredPartitions(ctx, []string{"metric_data_points"}, before, transaction.countRows)
		if err != nil {
			return err
		}
		return queries.DeleteSpanRollupBucketsBefore(ctx, startOfUTCDay(before))
	})
	if err != nil {
		return 0, 0, err
	}
	return events, points, nil
}

func (store *PartitionStore) ListRetentionProjects(ctx context.Context) ([]string, error) {
	projects, err := store.queries.ListProjectRetention(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	return ids, nil
}

// WithProjectRetention releases the policy lock only after the cleanup operation
// has been dispatched. A policy update and a worker cannot select different cutoffs concurrently.
func (store *PartitionStore) WithProjectRetention(ctx context.Context, projectID string, now time.Time, fallback time.Duration, work func(time.Time) error) error {
	return store.withProjectRetention(ctx, projectID, now, fallback, func(_ *sqlcstore.Queries, before time.Time) error {
		return work(before)
	})
}

func (store *PartitionStore) withProjectRetention(ctx context.Context, projectID string, now time.Time, fallback time.Duration, work func(*sqlcstore.Queries, time.Time) error) error {
	return pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		queries := store.queries.WithTx(tx)
		days, err := queries.LockProjectRetention(ctx, projectID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		age := telemetry.RetentionAge(days, fallback)
		if age == 0 {
			return nil
		}
		return work(queries, now.Add(-age))
	})
}

func (store *PartitionStore) PruneProjectTelemetry(ctx context.Context, projectID string, now time.Time, fallback time.Duration, batchSize int) (int64, int64, error) {
	var events, points int64
	for {
		var batchEvents, batchSpans, batchPoints int64
		err := store.withProjectRetention(ctx, projectID, now, fallback, func(queries *sqlcstore.Queries, before time.Time) error {
			var err error
			batchEvents, err = queries.DeleteProjectEventsBefore(ctx, sqlcstore.DeleteProjectEventsBeforeParams{ProjectID: projectID, BeforeTime: before, BatchSize: int32(batchSize)})
			if err != nil {
				return err
			}
			batchSpans, err = queries.DeleteProjectSpansBefore(ctx, sqlcstore.DeleteProjectSpansBeforeParams{ProjectID: projectID, BeforeTime: before, BatchSize: int32(batchSize)})
			if err != nil {
				return err
			}
			batchPoints, err = queries.DeleteProjectMetricsBefore(ctx, sqlcstore.DeleteProjectMetricsBeforeParams{ProjectID: projectID, BeforeTime: before, BatchSize: int32(batchSize)})
			return err
		})
		if err != nil {
			return events, points, err
		}
		events += batchEvents
		points += batchPoints
		if max(batchEvents, batchSpans, batchPoints) < int64(batchSize) {
			return events, points, nil
		}
	}
}

func (store *PartitionStore) CountExpiredProjectEvents(ctx context.Context, now time.Time, fallback time.Duration) (int64, error) {
	projects, err := store.queries.ListProjectRetention(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, project := range projects {
		if age := telemetry.RetentionAge(project.TelemetryRetentionDays, fallback); age > 0 {
			count, err := store.queries.CountProjectEventsBefore(ctx, sqlcstore.CountProjectEventsBeforeParams{ProjectID: project.ID, BeforeTime: now.Add(-age)})
			if err != nil {
				return 0, err
			}
			total += count
		}
	}
	return total, nil
}
