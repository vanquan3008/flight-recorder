package clickhouse

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

var detectionScope = config.IngestionConfig{ProjectID: testProject, EnvironmentID: testEnvironment}

func serverSpan(at time.Time, identity, service string, duration time.Duration, status string) telemetry.Event {
	span := testSpan(at, identity)
	span.TraceID = fmt.Sprintf("%032x", at.UnixNano())
	span.ServiceName, span.SpanKind, span.Duration, span.Status, span.Operation = service, 2, duration, status, "GET /orders"
	span.RecordAttributes = map[string]any{"http.route": "/orders", "http.request.method": "GET"}
	return span
}

func TestServiceAndDependencyDetectionIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	end := time.Now().UTC().Truncate(time.Minute)
	var spans []telemetry.Event
	for index := range 40 { // a steady, healthy baseline
		at := end.Add(-30*time.Minute + time.Duration(index)*30*time.Second)
		spans = append(spans, serverSpan(at, fmt.Sprint("baseline", index), "api", 10*time.Millisecond, "OK"))
		call := testSpan(at, fmt.Sprint("db-baseline", index))
		call.ServiceName, call.Duration, call.Status = "api", 10*time.Millisecond, "OK"
		spans = append(spans, call)
	}
	for index := range 20 { // the current window fails and its database calls slow down
		at := end.Add(-4*time.Minute + time.Duration(index)*10*time.Second)
		spans = append(spans, serverSpan(at, fmt.Sprint("current", index), "api", 10*time.Millisecond, "ERROR"))
		call := testSpan(at, fmt.Sprint("db-current", index))
		call.ServiceName, call.Duration, call.Status = "api", 200*time.Millisecond, "OK"
		spans = append(spans, call)
	}
	if err := store.StoreSpans(ctx, testProject, testEnvironment, spans); err != nil {
		t.Fatal(err)
	}
	services, err := store.DetectServiceEvidence(ctx, detectionScope, end)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, item := range services {
		found[item.Item.MetricName] = item.Service == "api" && len(item.Item.TraceIDs) > 0 && len(item.Item.TraceIDs) <= 5
	}
	if !found["error_rate"] {
		t.Fatalf("error rate rise not detected: %+v", services)
	}
	dependencies, err := store.DetectDependencyEvidence(ctx, detectionScope, end)
	if err != nil || len(dependencies) != 1 || dependencies[0].Item.EntityID != "database:orders" || dependencies[0].Item.MetricName != "latency" {
		t.Fatalf("dependency evidence = %+v, %v", dependencies, err)
	}
}

func samplePoint(at time.Time, name string, value float64, identity ...string) telemetry.MetricDataPoint {
	return telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, append([]string{"metric", name}, identity...)...), ProjectID: testProject,
		EnvironmentID: testEnvironment, ServiceName: "worker", MetricName: name, MetricType: telemetry.MetricTypeGauge, OccurredAt: at, Value: &value}
}

func TestMetricDetectionSeparatesDimensionsAndUsesCounterRatesIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	end := time.Now().UTC().Truncate(time.Minute)
	var points []telemetry.MetricDataPoint
	for _, region := range []string{"one", "two"} {
		for index, age := range []time.Duration{30 * time.Minute, 20 * time.Minute, 4 * time.Minute, 2 * time.Minute} {
			value := map[string][]float64{"one": {1, 1, 3, 3}, "two": {10, 10, 1, 1}}[region][index]
			point := samplePoint(end.Add(-age), "queue.backlog", value, region)
			point.Attributes = map[string]any{"region": region, "queue.name": "orders"}
			points = append(points, point)
		}
	}
	monotonic := true
	for index, age := range []time.Duration{33 * time.Minute, 23 * time.Minute, 13 * time.Minute, 4 * time.Minute, 3 * time.Minute, 2 * time.Minute} {
		point := samplePoint(end.Add(-age), "queue.processed", []float64{100, 200, 300, 305, 306, 307}[index])
		point.MetricType, point.Temporality, point.IsMonotonic, point.StartedAt = telemetry.MetricTypeSum, "cumulative", &monotonic, end.Add(-time.Hour)
		points = append(points, point)
	}
	if err := store.StoreMetrics(ctx, testProject, testEnvironment, points); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.DetectMetricEvidence(ctx, detectionScope, end)
	if err != nil || len(evidence) != 2 {
		t.Fatalf("evidence = %+v, %v", evidence, err)
	}
	foundBacklog, foundRate := false, false
	for _, item := range evidence {
		if item.Item.SeriesID == "" {
			t.Fatal("series identity missing")
		}
		switch item.Item.MetricName {
		case "queue.backlog":
			foundBacklog = item.Item.Direction == "increase" && item.Item.ObservedValue == 3 && item.Item.BaselineValue == 1 && item.Item.EntityID == "queue:orders"
		case "queue.processed":
			foundRate = item.Item.Direction == "decrease" && item.Item.ObservedValue < item.Item.BaselineValue*.5
		}
	}
	if !foundBacklog || !foundRate {
		t.Fatalf("incorrect dimension or rate evidence: %+v", evidence)
	}
}

func TestHistogramAndCounterResetDetectionIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	end := time.Now().UTC().Truncate(time.Minute)
	var points []telemetry.MetricDataPoint
	for index, age := range []time.Duration{30 * time.Minute, 25 * time.Minute, 20 * time.Minute, 4 * time.Minute, 2 * time.Minute} {
		point := samplePoint(end.Add(-age), "request.duration", 0)
		point.Value, point.MetricType, point.MetricUnit, point.Temporality = nil, telemetry.MetricTypeHistogram, "ms", "delta"
		point.StartedAt = point.OccurredAt.Add(-time.Minute)
		count, sum := []int64{100, 10000, 1, 100, 1}[index], []float64{1000, 0, 100, 1000, 200}[index]
		point.HistCount, point.HistSum = &count, &sum
		if index == 1 { // a histogram without a sum adds neither mean nor denominator
			point.HistSum = nil
		}
		points = append(points, point)
	}
	monotonic := true
	for index, age := range []time.Duration{33 * time.Minute, 23 * time.Minute, 13 * time.Minute, 4 * time.Minute, 2 * time.Minute} {
		point := samplePoint(end.Add(-age), "queue.processed", []float64{100, 200, 300, 1, 2}[index])
		point.MetricType, point.Temporality, point.IsMonotonic, point.StartedAt = telemetry.MetricTypeSum, "cumulative", &monotonic, end.Add(-time.Hour)
		if index >= 3 {
			point.StartedAt = end.Add(-5 * time.Minute)
		}
		points = append(points, point)
	}
	if err := store.StoreMetrics(ctx, testProject, testEnvironment, points); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.DetectMetricEvidence(ctx, detectionScope, end)
	if err != nil || len(evidence) != 1 || evidence[0].Item.MetricName != "queue.processed" || evidence[0].Item.Direction != "decrease" {
		t.Fatalf("histogram means must not alarm and a reset must read its new interval: %+v, %v", evidence, err)
	}
}

func TestRollupsServeLongPerformanceWindowsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	end := time.Now().UTC().Truncate(time.Minute)
	from := end.Add(-2 * time.Hour)
	var spans []telemetry.Event
	for index := range 120 {
		status := "OK"
		if index%10 == 0 {
			status = "ERROR"
		}
		spans = append(spans, serverSpan(from.Add(time.Duration(index)*time.Minute+time.Second), fmt.Sprint("rollup", index), "api", time.Duration(index+1)*time.Millisecond, status))
	}
	if err := store.StoreSpans(ctx, testProject, testEnvironment, spans); err != nil {
		t.Fatal(err)
	}
	filter := query.PerformanceFilter{EnvironmentID: testEnvironment, From: from, To: end, Limit: 10}
	raw, err := store.RequestPerformance(ctx, testProject, filter)
	if err != nil || raw.Summary.Requests != 120 || raw.Summary.Errors != 12 {
		t.Fatalf("performance before rollups = %+v, %v", raw.Summary, err)
	}
	for {
		count, err := store.RefreshSpanRollups(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			break
		}
	}
	if count, err := store.RefreshSpanRollups(ctx); err != nil || count != 0 {
		t.Fatalf("clean minutes were recomputed: %d, %v", count, err)
	}
	rolled, err := store.RequestPerformance(ctx, testProject, filter)
	if err != nil || rolled.Summary.Requests != 120 || rolled.Summary.Errors != 12 || len(rolled.Endpoints) != 1 || rolled.Endpoints[0].Endpoint != "GET /orders" {
		t.Fatalf("performance from rollups = %+v, %v", rolled, err)
	}
	if rolled.Summary.AverageLatencyNanos != raw.Summary.AverageLatencyNanos || rolled.Summary.P95LatencyNanos == 0 {
		t.Fatalf("rollups changed the averages: raw=%+v rolled=%+v", raw.Summary, rolled.Summary)
	}
	late := serverSpan(from.Add(30*time.Minute+30*time.Second), "late", "api", time.Millisecond, "OK")
	if err := store.StoreSpans(ctx, testProject, testEnvironment, []telemetry.Event{late}); err != nil {
		t.Fatal(err)
	}
	if again, err := store.RequestPerformance(ctx, testProject, filter); err != nil || again.Summary.Requests != 121 {
		t.Fatalf("a late span in a rolled-up minute was missed: %+v, %v", again.Summary, err)
	}
	if count, err := store.RefreshSpanRollups(ctx); err != nil || count != 1 {
		t.Fatalf("the late span's minute was not marked dirty: %d, %v", count, err)
	}
	if again, err := store.RequestPerformance(ctx, testProject, filter); err != nil || again.Summary.Requests != 121 {
		t.Fatalf("the recomputed minute changed the total: %+v, %v", again.Summary, err)
	}
}
