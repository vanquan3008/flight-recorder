package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// StoreMetrics persists an idempotent batch. SendBatch uses an implicit
// transaction; acknowledgement remains asynchronous on this best-effort path.
func (database *Database) StoreMetrics(ctx context.Context, ingestion config.IngestionConfig, points []telemetry.MetricDataPoint) error {
	if len(points) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, point := range points {
		if point.ProjectID != ingestion.ProjectID || point.EnvironmentID != ingestion.EnvironmentID {
			return errors.New("metric ingestion scope mismatch")
		}
		attrs, attrErr := marshalJSONObject(point.Attributes)
		if attrErr != nil {
			return fmt.Errorf("encode metric attributes: %w", attrErr)
		}
		resAttrs, resErr := marshalJSONObject(point.ResourceAttributes)
		if resErr != nil {
			return fmt.Errorf("encode metric resource attributes: %w", resErr)
		}
		scopeAttrs, scopeErr := marshalJSONObject(point.ScopeAttributes)
		if scopeErr != nil {
			return fmt.Errorf("encode metric scope attributes: %w", scopeErr)
		}
		exemplars, exemplarErr := marshalJSONArray(point.Exemplars)
		if exemplarErr != nil {
			return fmt.Errorf("encode metric exemplars: %w", exemplarErr)
		}

		buckets, bucketsErr := encodeHistogramBuckets(point.HistBuckets)
		if bucketsErr != nil {
			return fmt.Errorf("encode histogram buckets: %w", bucketsErr)
		}

		batch.Queue(`
			INSERT INTO metric_data_points (
				id, project_id, environment_id,
				metric_name, metric_unit, metric_type,
				service_name, service_version,
				occurred_at,
				value, hist_count, hist_sum, hist_min, hist_max, hist_buckets,
				temporality, is_monotonic,
				attributes, resource_attributes,
				scope_name, scope_version, exemplars, started_at, flags, scope_attributes
			) VALUES (
				$1, $2, $3,
				$4, $5, $6,
				$7, $8,
				$9,
				$10, $11, $12, $13, $14, $15,
				$16, $17,
				$18, $19,
				$20, $21, $22, $23, $24, $25
			) ON CONFLICT (project_id, id, occurred_at) DO NOTHING`,
			point.ID, ingestion.ProjectID, ingestion.EnvironmentID,
			point.MetricName, point.MetricUnit, string(point.MetricType),
			point.ServiceName, nullableString(point.ServiceVersion),
			point.OccurredAt,
			nullableFloat64(point.Value), nullableInt64Ptr(point.HistCount), nullableFloat64(point.HistSum), nullableFloat64(point.HistMin), nullableFloat64(point.HistMax), nullableBytesOrNil(buckets),
			point.Temporality, point.IsMonotonic,
			attrs, resAttrs,
			point.ScopeName, point.ScopeVersion, exemplars, nullableTime(point.StartedAt), int64(point.Flags), scopeAttrs,
		)
	}

	results := database.pool.SendBatch(ctx, batch)
	for range len(points) {
		if _, execErr := results.Exec(); execErr != nil {
			_ = results.Close()
			return fmt.Errorf("insert metric data point: %w", execErr)
		}
	}
	if closeError := results.Close(); closeError != nil {
		return fmt.Errorf("close metric batch: %w", closeError)
	}
	return nil
}

// DeleteMetricsBefore expires metric data points the same way as events: whole
// daily partitions are dropped and the default partition is trimmed in batches.
func (database *Database) DeleteMetricsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	dropped, dropError := database.dropExpiredPartitions(ctx, []string{"metric_data_points"}, before)
	if dropError != nil {
		return dropped, fmt.Errorf("drop expired metric partitions: %w", dropError)
	}
	deleted, deleteError := database.deleteDefaultPartitionRows(ctx, "metric_data_points", before, batchSize)
	return dropped + deleted, deleteError
}

// nullableFloat64 returns nil for nil pointers or non-finite values.
func nullableFloat64(v *float64) any {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	return *v
}

func nullableInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableBytesOrNil(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
