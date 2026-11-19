package clickhouse

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestSpansAndMetricsDeduplicateRetriesAndExpireByDay(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	now := time.Now().UTC()
	old, recent := testSpan(now.Add(-72*time.Hour), "old-span"), testSpan(now, "recent-span")
	old.TraceID = "11111111111111111111111111111111"
	value := 1.0
	point := telemetry.MetricDataPoint{ID: telemetry.DeterministicID(now, "point"), ProjectID: testProject, EnvironmentID: testEnvironment,
		MetricName: "requests", MetricType: telemetry.MetricTypeGauge, ServiceName: "api", OccurredAt: now.Add(-72 * time.Hour), Value: &value}
	for range 2 { // an exporter retry repeats the whole batch
		if err := store.StoreSpans(ctx, testProject, testEnvironment, []telemetry.Event{old, recent}); err != nil {
			t.Fatal(err)
		}
		if err := store.StoreMetrics(ctx, testProject, testEnvironment, []telemetry.MetricDataPoint{point}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(sql string) int64 {
		t.Helper()
		data, err := store.execute(ctx, sql+" FORMAT TabSeparated", nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	counts := func() (spans, traces, points int64) {
		return count("SELECT count() FROM tracepilot_spans FINAL"), count("SELECT uniqExact(trace_id) FROM tracepilot_trace_index"),
			count("SELECT count() FROM tracepilot_metric_points FINAL")
	}
	if spans, traces, points := counts(); spans != 2 || traces != 2 || points != 1 {
		t.Fatalf("retries changed counts: spans=%d traces=%d points=%d", spans, traces, points)
	}
	if failed := count("SELECT countIf(request_failed AND dependency_type = 'database') FROM tracepilot_spans FINAL"); failed != 2 {
		t.Fatalf("derived span columns were not stored: %d", failed)
	}
	if refreshed, err := store.RefreshSpanRollups(ctx); err != nil || refreshed == 0 {
		t.Fatalf("rollups of the old minute: %d, %v", refreshed, err)
	}
	expiredRollups := func() int64 {
		t.Helper()
		var total int64
		for _, table := range []string{"tracepilot_span_minutes", "tracepilot_span_minute_marks", "tracepilot_span_minute_done"} {
			total += count("SELECT count() FROM " + table + " FINAL WHERE minute < now() - INTERVAL 1 DAY")
		}
		return total
	}
	if expiredRollups() != 3 {
		t.Fatalf("the old minute has %d rollup rows, want one per rollup table", expiredRollups())
	}
	if err := store.ExpireProjectTelemetry(ctx, testProject, now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if spans, traces, points := counts(); spans != 1 || traces != 1 || points != 0 || expiredRollups() != 0 {
		t.Fatalf("expired rows remain: spans=%d traces=%d points=%d rollups=%d", spans, traces, points, expiredRollups())
	}
	if err := store.DropPartitionsBefore(ctx, startOfUTCDay(now)); err != nil {
		t.Fatal(err)
	}
	monday := startOfUTCDay(now).AddDate(0, 0, -(int(now.Weekday())+6)%7)
	if parts := count("SELECT count() FROM system.parts WHERE database = currentDatabase() AND table = 'tracepilot_spans' AND active AND partition_id < '" +
		monday.Format("20060102") + "'"); parts != 0 {
		t.Fatalf("%d expired span parts remain after dropping partitions", parts)
	}
	if spans, _, _ := counts(); spans != 1 {
		t.Fatalf("dropping expired partitions removed current spans: %d", spans)
	}
	if err := store.Ready(ctx); err != nil {
		t.Fatalf("a migrated store failed readiness: %v", err)
	}
}
