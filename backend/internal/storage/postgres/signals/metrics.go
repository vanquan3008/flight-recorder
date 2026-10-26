package signals

import (
	"context"
	"errors"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgbatch"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// StoreMetrics persists an idempotent batch in one implicit transaction, so the
// pipeline acknowledges only committed points. Errors caused by point content
// wrap telemetry.ErrInvalidEvent.
func (store *SignalStore) StoreMetrics(ctx context.Context, ingestion config.IngestionConfig, points []telemetry.MetricDataPoint) error {
	return markInvalidEvents(store.storeMetrics(ctx, ingestion, points))
}

func (store *SignalStore) storeMetrics(ctx context.Context, ingestion config.IngestionConfig, points []telemetry.MetricDataPoint) error {
	if len(points) == 0 {
		return nil
	}
	queue := &pgbatch.Queue{}
	writes := sqlcstore.New(queue)
	for _, point := range points {
		if point.ProjectID != ingestion.ProjectID || point.EnvironmentID != ingestion.EnvironmentID {
			return errors.New("metric ingestion scope mismatch")
		}
		if queueError := queueMetricPoint(ctx, writes, ingestion, point); queueError != nil {
			return queueError
		}
	}
	if sendError := queue.Send(ctx, store.pool); sendError != nil {
		return fmt.Errorf("insert metric data points: %w", sendError)
	}
	return nil
}

func queueMetricPoint(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, point telemetry.MetricDataPoint) error {
	attributes, attributesError := pgvalue.MarshalJSONObject(point.Attributes)
	if attributesError != nil {
		return fmt.Errorf("encode metric attributes: %w", attributesError)
	}
	resourceAttributes, resourceError := pgvalue.MarshalJSONObject(point.ResourceAttributes)
	if resourceError != nil {
		return fmt.Errorf("encode metric resource attributes: %w", resourceError)
	}
	scopeAttributes, scopeError := pgvalue.MarshalJSONObject(point.ScopeAttributes)
	if scopeError != nil {
		return fmt.Errorf("encode metric scope attributes: %w", scopeError)
	}
	exemplars, exemplarError := pgvalue.MarshalJSONArray(point.Exemplars)
	if exemplarError != nil {
		return fmt.Errorf("encode metric exemplars: %w", exemplarError)
	}
	buckets, bucketsError := encodeHistogramBuckets(point.HistBuckets)
	if bucketsError != nil {
		return fmt.Errorf("encode histogram buckets: %w", bucketsError)
	}
	writes.InsertMetricPoint(ctx, sqlcstore.InsertMetricPointParams{
		ID: point.ID, ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		MetricName: point.MetricName, MetricUnit: point.MetricUnit, MetricType: string(point.MetricType),
		ServiceName: point.ServiceName, ServiceVersion: pgvalue.OrNil(point.ServiceVersion), OccurredAt: point.OccurredAt,
		Value: pgvalue.FiniteOrNil(point.Value), HistCount: point.HistCount,
		HistSum: pgvalue.FiniteOrNil(point.HistSum), HistMin: pgvalue.FiniteOrNil(point.HistMin), HistMax: pgvalue.FiniteOrNil(point.HistMax),
		HistBuckets: pgvalue.BytesOrNil(buckets), Temporality: point.Temporality, IsMonotonic: point.IsMonotonic,
		Attributes: attributes, ResourceAttributes: resourceAttributes, ScopeAttributes: scopeAttributes,
		ScopeName: point.ScopeName, ScopeVersion: point.ScopeVersion, Exemplars: exemplars,
		StartedAt: pgvalue.TimeOrNil(point.StartedAt), Flags: int64(point.Flags),
	})
	return nil
}
