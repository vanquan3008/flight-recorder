package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const metricColumns = `id, series_id, metric_name, metric_unit, metric_type, service_name, service_version,
occurred_at, started_at, flags, value, hist_count, hist_sum, hist_min, hist_max, hist_buckets, temporality,
is_monotonic, attributes, resource_attributes, scope_name, scope_version, scope_attributes, exemplars`

type storedMetricPoint struct {
	ProjectID          string            `json:"project_id"`
	EnvironmentID      string            `json:"environment_id"`
	ID                 string            `json:"id"`
	SeriesID           string            `json:"series_id"`
	MetricName         string            `json:"metric_name"`
	MetricUnit         string            `json:"metric_unit"`
	MetricType         string            `json:"metric_type"`
	ServiceName        string            `json:"service_name"`
	ServiceVersion     string            `json:"service_version"`
	OccurredAt         time.Time         `json:"occurred_at"`
	StartedAt          *time.Time        `json:"started_at"`
	Flags              uint32            `json:"flags"`
	Value              *float64          `json:"value"`
	HistCount          *int64            `json:"hist_count"`
	HistSum            *float64          `json:"hist_sum"`
	HistMin            *float64          `json:"hist_min"`
	HistMax            *float64          `json:"hist_max"`
	HistBuckets        string            `json:"hist_buckets"`
	Temporality        string            `json:"temporality"`
	IsMonotonic        *bool             `json:"is_monotonic"`
	Attributes         map[string]string `json:"attributes"`
	ResourceAttributes map[string]string `json:"resource_attributes"`
	ScopeName          string            `json:"scope_name"`
	ScopeVersion       string            `json:"scope_version"`
	ScopeAttributes    map[string]string `json:"scope_attributes"`
	Exemplars          string            `json:"exemplars"`
}

func (store *Store) StoreMetrics(ctx context.Context, projectID, environmentID string, points []telemetry.MetricDataPoint) error {
	if err := validateScope(projectID, environmentID); err != nil {
		return err
	}
	if len(points) == 0 {
		return nil
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	for _, point := range points {
		if point.ProjectID != projectID || point.EnvironmentID != environmentID {
			return fmt.Errorf("ClickHouse metric ingestion scope mismatch: %w", telemetry.ErrInvalidEvent)
		}
		row, err := metricPoint(point)
		if err != nil {
			return err
		}
		if err := encoder.Encode(row); err != nil {
			return fmt.Errorf("encode ClickHouse metric point: %w", telemetry.ErrInvalidEvent)
		}
	}
	_, err := store.execute(ctx, "INSERT INTO tracepilot_metric_points FORMAT JSONEachRow", nil, payload.Bytes(), true)
	return err
}

func metricPoint(point telemetry.MetricDataPoint) (storedMetricPoint, error) {
	switch point.MetricType {
	case telemetry.MetricTypeGauge, telemetry.MetricTypeSum, telemetry.MetricTypeHistogram:
	default:
		return storedMetricPoint{}, fmt.Errorf("unsupported metric type: %w", telemetry.ErrInvalidEvent)
	}
	if !validUUID(point.ID) || point.OccurredAt.IsZero() {
		return storedMetricPoint{}, fmt.Errorf("invalid metric identity or timestamp: %w", telemetry.ErrInvalidEvent)
	}
	row := storedMetricPoint{
		ProjectID: point.ProjectID, EnvironmentID: point.EnvironmentID, ID: point.ID, SeriesID: point.SeriesID(),
		MetricName: point.MetricName, MetricUnit: point.MetricUnit, MetricType: string(point.MetricType),
		ServiceName: point.ServiceName, ServiceVersion: point.ServiceVersion, OccurredAt: point.OccurredAt.UTC(), Flags: point.Flags,
		Value: finiteOrNil(point.Value), HistCount: point.HistCount, HistSum: finiteOrNil(point.HistSum),
		HistMin: finiteOrNil(point.HistMin), HistMax: finiteOrNil(point.HistMax),
		Temporality: point.Temporality, IsMonotonic: point.IsMonotonic, ScopeName: point.ScopeName, ScopeVersion: point.ScopeVersion,
	}
	if !point.StartedAt.IsZero() {
		started := point.StartedAt.UTC()
		row.StartedAt = &started
	}
	var err error
	if row.HistBuckets, err = histogramBuckets(point.HistBuckets); err != nil {
		return row, err
	}
	if row.Attributes, err = attributeMap(point.Attributes); err != nil {
		return row, err
	}
	if row.ResourceAttributes, err = attributeMap(point.ResourceAttributes); err != nil {
		return row, err
	}
	if row.ScopeAttributes, err = attributeMap(point.ScopeAttributes); err != nil {
		return row, err
	}
	row.Exemplars, err = jsonArray(point.Exemplars)
	return row, err
}

// histogramBuckets keeps the API shape: the +Inf bucket has a null bound.
func histogramBuckets(buckets []telemetry.HistogramBucket) (string, error) {
	if len(buckets) == 0 {
		return "", nil
	}
	type bucket struct {
		Bound *float64 `json:"bound"`
		Count uint64   `json:"count"`
	}
	encoded := make([]bucket, len(buckets))
	for index, item := range buckets {
		if math.IsNaN(item.UpperBound) || math.IsInf(item.UpperBound, -1) {
			return "", fmt.Errorf("histogram bound is invalid: %w", telemetry.ErrInvalidEvent)
		}
		encoded[index].Count = item.Count
		if !math.IsInf(item.UpperBound, 1) {
			bound := item.UpperBound
			encoded[index].Bound = &bound
		}
	}
	return jsonText(encoded)
}

func finiteOrNil(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	return value
}
