package postgres

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
func (database *Database) DeleteEventsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	dropped, dropError := database.dropExpiredPartitions(ctx, eventPartitionedTables, before)
	if dropError != nil {
		return dropped, fmt.Errorf("drop expired event partitions: %w", dropError)
	}
	deleted, deleteError := database.deleteDefaultPartitionRows(ctx, "events", before, batchSize)
	if deleteError != nil {
		return dropped + deleted, deleteError
	}
	// Spans and logs in the default partition expire with the same cutoff.
	for _, table := range []string{"spans", "logs"} {
		if _, tableError := database.deleteDefaultPartitionRows(ctx, table, before, batchSize); tableError != nil {
			return dropped + deleted, tableError
		}
	}
	return dropped + deleted, nil
}

// deleteDefaultPartitionRows removes up to batchSize expired rows from the
// default partition of table. Only that partition is touched: rows in a dated
// partition are removed by dropping the partition, not by DELETE.
func (database *Database) deleteDefaultPartitionRows(ctx context.Context, table string, before time.Time, batchSize int) (int64, error) {
	timeColumn := "occurred_at"
	if table == "spans" {
		timeColumn = "started_at"
	}
	result, deleteError := database.pool.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %[1]s_default WHERE ctid IN (SELECT ctid FROM %[1]s_default WHERE %[2]s < $1 LIMIT $2)",
		table, timeColumn), before, batchSize)
	if deleteError != nil {
		return 0, fmt.Errorf("delete expired %s: %w", table, deleteError)
	}
	return result.RowsAffected(), nil
}

func (database *Database) CountEventsBefore(ctx context.Context, before time.Time) (int64, error) {
	var count int64
	if queryError := database.pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE occurred_at < $1", before).Scan(&count); queryError != nil {
		return 0, fmt.Errorf("count expired events: %w", queryError)
	}
	return count, nil
}
