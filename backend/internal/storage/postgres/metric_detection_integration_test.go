package postgres

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestHistogramBatchPersistsUnboundedBucketsAndSampleMetadata(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	gauge := metricSample(scope, "queue.depth", now, 1)
	hist := metricSample(scope, "request.duration", now, 0)
	hist.Value = nil
	hist.MetricType = telemetry.MetricTypeHistogram
	hist.MetricUnit = "ms"
	count, sum := int64(3), 12.0
	hist.HistCount, hist.HistSum = &count, &sum
	hist.HistBuckets = []telemetry.HistogramBucket{{UpperBound: 10, Count: 2}, {UpperBound: math.Inf(1), Count: 1}}
	hist.StartedAt = now.Add(-time.Minute)
	hist.ScopeAttributes = map[string]any{"version": "one"}
	for range 2 {
		if err := database.StoreMetrics(context.Background(), scope, []telemetry.MetricDataPoint{gauge, hist}); err != nil {
			t.Fatal(err)
		}
	}
	var rows int
	var buckets string
	var start time.Time
	if err := database.pool.QueryRow(context.Background(), `SELECT count(*) FROM metric_data_points WHERE project_id=$1`, scope.ProjectID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(context.Background(), `SELECT hist_buckets::text,started_at FROM metric_data_points WHERE project_id=$1 AND id=$2`, scope.ProjectID, hist.ID).Scan(&buckets, &start); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || !start.Equal(hist.StartedAt) || buckets == "" {
		t.Fatalf("rows=%d buckets=%s start=%s", rows, buckets, start)
	}
}

func TestMetricDetectionSeparatesDimensionsAndUsesCounterRates(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	end := time.Now().UTC().Truncate(time.Minute)
	points := make([]telemetry.MetricDataPoint, 0)
	for _, region := range []string{"one", "two"} {
		for index, age := range []time.Duration{30 * time.Minute, 20 * time.Minute, 4 * time.Minute, 2 * time.Minute} {
			value := 1.0
			if region == "one" && index >= 2 {
				value = 3
			}
			if region == "two" {
				value = 10
				if index >= 2 {
					value = 1
				}
			}
			point := metricSample(scope, "queue.backlog", end.Add(-age), value)
			point.Attributes = map[string]any{"region": region, "queue.name": "orders"}
			point.ID = telemetry.DeterministicID(point.OccurredAt, "dimension-sample", scope.ProjectID, region)
			points = append(points, point)
		}
	}
	monotonic := true
	for index, age := range []time.Duration{33 * time.Minute, 23 * time.Minute, 13 * time.Minute, 4 * time.Minute, 3 * time.Minute, 2 * time.Minute} {
		point := metricSample(scope, "queue.processed", end.Add(-age), []float64{100, 200, 300, 305, 306, 307}[index])
		point.MetricType, point.Temporality, point.IsMonotonic = telemetry.MetricTypeSum, "cumulative", &monotonic
		point.StartedAt = end.Add(-time.Hour)
		points = append(points, point)
	}
	if err := database.StoreMetrics(context.Background(), scope, points); err != nil {
		t.Fatal(err)
	}
	evidence, err := database.DetectMetricEvidence(context.Background(), scope, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 2 {
		t.Fatalf("evidence=%#v", evidence)
	}
	foundBacklog, foundRate := false, false
	for _, item := range evidence {
		if item.Item.SeriesID == "" {
			t.Fatal("series identity missing")
		}
		if item.Item.MetricName == "queue.backlog" {
			foundBacklog = item.Item.Direction == "increase" && item.Item.ObservedValue == 3 && item.Item.BaselineValue == 1
		}
		if item.Item.MetricName == "queue.processed" {
			foundRate = item.Item.Direction == "decrease" && item.Item.ObservedValue < item.Item.BaselineValue*.5
		}
	}
	if !foundBacklog || !foundRate {
		t.Fatalf("incorrect dimension/rate evidence=%#v", evidence)
	}
}

func TestHistogramDetectionUsesObservationWeightedMean(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	end := time.Now().UTC().Truncate(time.Minute)
	points := make([]telemetry.MetricDataPoint, 0)
	for index, age := range []time.Duration{30 * time.Minute, 20 * time.Minute, 4 * time.Minute, 2 * time.Minute} {
		point := metricSample(scope, "request.duration", end.Add(-age), 0)
		point.Value = nil
		point.MetricType, point.MetricUnit, point.Temporality = telemetry.MetricTypeHistogram, "ms", "delta"
		point.StartedAt = point.OccurredAt.Add(-time.Minute)
		count, sum := []int64{100, 1, 100, 1}[index], []float64{1000, 100, 1000, 200}[index]
		point.HistCount, point.HistSum = &count, &sum
		points = append(points, point)
	}
	// A histogram without a sum cannot contribute a mean or its denominator.
	missing := metricSample(scope, "request.duration", end.Add(-25*time.Minute), 0)
	missing.Value = nil
	missing.MetricType, missing.MetricUnit, missing.Temporality = telemetry.MetricTypeHistogram, "ms", "delta"
	missing.StartedAt = missing.OccurredAt.Add(-time.Minute)
	missingCount := int64(10000)
	missing.HistCount = &missingCount
	points = append(points, missing)
	if err := database.StoreMetrics(context.Background(), scope, points); err != nil {
		t.Fatal(err)
	}
	evidence, err := database.DetectMetricEvidence(context.Background(), scope, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 0 {
		t.Fatalf("averaging histogram means caused false anomaly: %#v", evidence)
	}
}

func metricSample(scope config.IngestionConfig, name string, at time.Time, value float64) telemetry.MetricDataPoint {
	return telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, "metric-regression", scope.ProjectID, name), ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, ServiceName: "worker", MetricName: name, MetricType: telemetry.MetricTypeGauge, OccurredAt: at, Value: &value, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}}
}

func TestCumulativeCounterResetUsesNewStartTime(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	end := time.Now().UTC().Truncate(time.Minute)
	monotonic := true
	points := make([]telemetry.MetricDataPoint, 0)
	for index, age := range []time.Duration{33 * time.Minute, 23 * time.Minute, 13 * time.Minute, 4 * time.Minute, 2 * time.Minute} {
		point := metricSample(scope, "queue.processed", end.Add(-age), []float64{100, 200, 300, 1, 2}[index])
		point.MetricType, point.Temporality, point.IsMonotonic = telemetry.MetricTypeSum, "cumulative", &monotonic
		point.StartedAt = end.Add(-time.Hour)
		if index >= 3 {
			point.StartedAt = end.Add(-5 * time.Minute)
		}
		points = append(points, point)
	}
	if err := database.StoreMetrics(context.Background(), scope, points); err != nil {
		t.Fatal(err)
	}
	evidence, err := database.DetectMetricEvidence(context.Background(), scope, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].Item.Direction != "decrease" {
		t.Fatalf("reset interval ignored/misread: %#v", evidence)
	}
}
