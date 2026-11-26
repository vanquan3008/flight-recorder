package metrics

import (
	"context"
	"errors"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
)

func TestStoreMetricsStoresNonFiniteValuesAsNullAndKeepsTheSeriesID(t *testing.T) {
	at := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	notANumber, count, sum := math.NaN(), int64(3), 4.5
	gauge := telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, "gauge"), ProjectID: telemetrytest.Project, EnvironmentID: telemetrytest.Environment,
		MetricName: "queue.depth", MetricType: telemetry.MetricTypeGauge, ServiceName: "api", OccurredAt: at, Value: &notANumber}
	histogram := telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, "histogram"), ProjectID: telemetrytest.Project, EnvironmentID: telemetrytest.Environment,
		MetricName: "latency", MetricType: telemetry.MetricTypeHistogram, ServiceName: "api", OccurredAt: at, StartedAt: at.Add(-time.Minute),
		HistCount: &count, HistSum: &sum, HistBuckets: []telemetry.HistogramBucket{{UpperBound: 1, Count: 2}, {UpperBound: math.Inf(1), Count: 1}}}
	var rows []storedMetricPoint
	store := New(clienttest.Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "INSERT INTO tracepilot_metric_points FORMAT JSONEachRow" {
			t.Errorf("unexpected query %q", r.URL.Query().Get("query"))
		}
		rows = clienttest.DecodeRows[storedMetricPoint](t, r)
	}))
	if err := store.StoreMetrics(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.MetricDataPoint{gauge, histogram}); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Value != nil || rows[0].StartedAt != nil || rows[0].SeriesID != gauge.SeriesID() || rows[0].HistBuckets != "" ||
		rows[0].Attributes == nil || len(rows[0].Attributes) != 0 {
		t.Fatalf("gauge row: %+v", rows)
	}
	if rows[1].HistBuckets != `[{"bound":1,"count":2},{"bound":null,"count":1}]` || rows[1].StartedAt == nil || *rows[1].HistCount != 3 {
		t.Fatalf("histogram row: %+v", rows[1])
	}
	bad := gauge
	bad.MetricType = "summary"
	if err := store.StoreMetrics(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.MetricDataPoint{bad}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("an unsupported metric type was accepted: %v", err)
	}
}
