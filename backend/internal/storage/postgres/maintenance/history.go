package maintenance

import (
	"context"
	"fmt"
	"time"

	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// DeleteIncidentsBefore removes up to batchSize resolved or ignored incidents
// with no activity since before, together with their evidence and history.
func (store *PartitionStore) DeleteIncidentsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, deleteError := store.queries.DeleteExpiredIncidents(ctx, sqlcstore.DeleteExpiredIncidentsParams{Before: before, BatchSize: int32(batchSize)})
	if deleteError != nil {
		return 0, fmt.Errorf("delete expired incidents: %w", deleteError)
	}
	return deleted, nil
}

// DeleteAuditBefore removes up to batchSize API key audit events older than before.
func (store *PartitionStore) DeleteAuditBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, deleteError := store.queries.DeleteExpiredAPIKeyEvents(ctx, sqlcstore.DeleteExpiredAPIKeyEventsParams{Before: before, BatchSize: int32(batchSize)})
	if deleteError != nil {
		return 0, fmt.Errorf("delete expired audit events: %w", deleteError)
	}
	return deleted, nil
}

// DeleteDeliveriesBefore removes up to batchSize finished webhook deliveries
// last updated before before. Pending and sending deliveries are kept.
func (store *PartitionStore) DeleteDeliveriesBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, deleteError := store.queries.DeleteExpiredWebhookDeliveries(ctx, sqlcstore.DeleteExpiredWebhookDeliveriesParams{Before: before, BatchSize: int32(batchSize)})
	if deleteError != nil {
		return 0, fmt.Errorf("delete expired webhook deliveries: %w", deleteError)
	}
	return deleted, nil
}
