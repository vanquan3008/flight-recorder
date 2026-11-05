package analytics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// RefreshSpanRollups replaces a bounded set of dirty minutes. Ingestion and
// refresh lock the same bucket rows, so late arrivals cannot be marked clean
// without being included. Readers fall back to spans while a bucket is dirty.
func (store *AnalyticsStore) RefreshSpanRollups(ctx context.Context, scope config.IngestionConfig) error {
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		queries := store.queries.WithTx(tx)
		minutes, err := queries.LockPendingSpanRollupBuckets(ctx, sqlcstore.LockPendingSpanRollupBucketsParams{
			ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, RowLimit: 64,
		})
		if err != nil || len(minutes) == 0 {
			return err
		}
		return queries.RefreshSpanRollups(ctx, sqlcstore.RefreshSpanRollupsParams{
			ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, BucketStarts: minutes,
		})
	})
	if err != nil {
		return fmt.Errorf("refresh span rollups: %w", err)
	}
	return nil
}
