package maintenance

import (
	"context"
	"fmt"
	"time"
)

// DeleteEventsBefore expires events, spans, and logs by dropping every daily
// partition that ended at or before the cutoff, so retention granularity is one
// UTC day and the cost does not grow with the number of rows. Rows that missed
// their daily partition live in events_default and are deleted in batches. It
// returns the number of events removed.
func (store *PartitionStore) DeleteEventsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	dropped, dropError := store.dropExpiredPartitions(ctx, eventPartitionedTables, before, store.countRows)
	if dropError != nil {
		return dropped, fmt.Errorf("drop expired event partitions: %w", dropError)
	}
	deleted, deleteError := store.deleteDefaultPartitionRows(ctx, "events", before, batchSize)
	if deleteError != nil {
		return dropped + deleted, deleteError
	}
	// Spans and logs in the default partition expire with the same cutoff.
	for _, table := range []string{"spans", "logs"} {
		if _, tableError := store.deleteDefaultPartitionRows(ctx, table, before, batchSize); tableError != nil {
			return dropped + deleted, tableError
		}
	}
	return dropped + deleted, nil
}

// deleteDefaultPartitionRows removes up to batchSize expired rows from the
// default partition of table. Only that partition is touched: rows in a dated
// partition are removed by dropping the partition, not by DELETE.
func (store *PartitionStore) deleteDefaultPartitionRows(ctx context.Context, table string, before time.Time, batchSize int) (int64, error) {
	timeColumn := "occurred_at"
	if table == "spans" {
		timeColumn = "started_at"
	}
	result, deleteError := store.pool.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %[1]s WHERE ctid IN (SELECT ctid FROM %[1]s WHERE %[2]s < $1 LIMIT $2)",
		identifier(table+"_default"), identifier(timeColumn)), before, batchSize)
	if deleteError != nil {
		return 0, fmt.Errorf("delete expired %s: %w", table, deleteError)
	}
	return result.RowsAffected(), nil
}

func (store *PartitionStore) CountEventsBefore(ctx context.Context, before time.Time) (int64, error) {
	count, queryError := store.queries.CountEventsBefore(ctx, before)
	if queryError != nil {
		return 0, fmt.Errorf("count expired events: %w", queryError)
	}
	return count, nil
}

// DeleteMetricsBefore expires metric data points the same way as events: whole
// daily partitions are dropped and the default partition is trimmed in batches.
func (store *PartitionStore) DeleteMetricsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	dropped, dropError := store.dropExpiredPartitions(ctx, []string{"metric_data_points"}, before, store.countRows)
	if dropError != nil {
		return dropped, fmt.Errorf("drop expired metric partitions: %w", dropError)
	}
	deleted, deleteError := store.deleteDefaultPartitionRows(ctx, "metric_data_points", before, batchSize)
	return dropped + deleted, deleteError
}
