package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeleteEventsBefore expires incident envelopes by dropping every daily
// partition that ended at or before the cutoff, so retention granularity is one
// UTC day and the cost does not grow with the number of rows. Rows that missed
// their daily partition live in events_default and are deleted in batches. It
// returns the number of events removed.
func (store *PartitionStore) DeleteEventsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	var deleted int64
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		transactionStore := &PartitionStore{pool: tx, queries: store.queries.WithTx(tx)}
		var deleteError error
		deleted, deleteError = transactionStore.deleteEventsBefore(ctx, before, batchSize)
		return deleteError
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (store *PartitionStore) deleteEventsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	dropped, dropError := store.dropExpiredPartitions(ctx, partitionedTables, before, store.countRows)
	if dropError != nil {
		return dropped, fmt.Errorf("drop expired event partitions: %w", dropError)
	}
	deleted, deleteError := store.deleteDefaultPartitionRows(ctx, "events", before, batchSize)
	return dropped + deleted, deleteError
}

// deleteDefaultPartitionRows removes up to batchSize expired rows from the
// default partition of table. Only that partition is touched: rows in a dated
// partition are removed by dropping the partition, not by DELETE.
func (store *PartitionStore) deleteDefaultPartitionRows(ctx context.Context, table string, before time.Time, batchSize int) (int64, error) {
	result, deleteError := store.pool.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %[1]s WHERE ctid IN (SELECT ctid FROM %[1]s WHERE occurred_at < $1 LIMIT $2)",
		identifier(table+"_default")), before, batchSize)
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
