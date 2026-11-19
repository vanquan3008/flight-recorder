package clickhouse

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func decodeRows[T any](t *testing.T, r *http.Request) []T {
	t.Helper()
	reader, err := gzip.NewReader(r.Body)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer reader.Close()
	var rows []T
	decoder := json.NewDecoder(reader)
	for decoder.More() {
		var row T
		if err := decoder.Decode(&row); err != nil {
			t.Error(err)
			return rows
		}
		rows = append(rows, row)
	}
	return rows
}

func testSpan(at time.Time, identity string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, identity), ProjectID: testProject, EnvironmentID: testEnvironment,
		ServiceName: "api", Signal: telemetry.SignalTrace, Type: telemetry.EventHTTPClient, Timestamp: at, Duration: 1500 * time.Microsecond,
		TraceID: "0123456789abcdef0123456789abcdef", SpanID: "0123456789abcdef", Operation: "SELECT orders", SpanKind: 3, Status: "ERROR",
		RecordAttributes: map[string]any{"db.system": "postgresql", "db.namespace": "orders"},
		SpanEvents:       []telemetry.SpanEvent{{Timestamp: at, Name: "exception", Attributes: map[string]any{"exception.type": "Timeout"}}},
	}
}

func TestStoreSpansWritesDerivedColumnsAndKeepsTheEventShape(t *testing.T) {
	at := time.Date(2026, 11, 19, 8, 0, 0, 123456789, time.UTC)
	var rows []storedSpan
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("query") != "INSERT INTO tracepilot_spans FORMAT JSONEachRow" || q.Get("async_insert") != "1" || q.Get("wait_for_async_insert") != "1" {
			t.Errorf("span insert must wait for the async insert to flush: %v", q)
		}
		rows = decodeRows[storedSpan](t, r)
	})
	if err := store.StoreSpans(context.Background(), testProject, testEnvironment, []telemetry.Event{testSpan(at, "span")}); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	row := rows[0]
	if !row.EndedAt.Equal(at.Add(1500*time.Microsecond)) || row.DurationNanos != 1_500_000 || !row.RequestFailed ||
		row.Endpoint != "SELECT orders" || row.DependencyType != "database" || row.DependencyName != "orders" || row.ParentSpanID != "" {
		t.Fatalf("derived columns: %+v", row)
	}
	if !strings.Contains(row.SpanEvents, `"Name":"exception"`) || row.SpanLinks != "[]" {
		t.Fatalf("span events and links must keep the PostgreSQL JSON shape: events=%s links=%s", row.SpanEvents, row.SpanLinks)
	}
	if row.SpanAttributes["db.system"] != "postgresql" || row.SpanAttributes["db.namespace"] != "orders" || len(row.ScopeAttributes) != 0 {
		t.Fatalf("span attributes must be a text map: %+v", row.SpanAttributes)
	}
}

func TestStoreSpansRejectsAnotherScopeBeforeWriting(t *testing.T) {
	store := testStore(t, func(http.ResponseWriter, *http.Request) { t.Error("a mismatched batch reached ClickHouse") })
	span := testSpan(time.Now().UTC(), "foreign")
	span.ProjectID = "01900000-0000-7000-8000-000000000099"
	if err := store.StoreSpans(context.Background(), testProject, testEnvironment, []telemetry.Event{span}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("want a permanent content failure, got %v", err)
	}
	log := testLog(time.Now().UTC(), "log")
	if err := store.StoreSpans(context.Background(), testProject, testEnvironment, []telemetry.Event{log}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("a log was accepted as a span: %v", err)
	}
}

func TestStoreMetricsStoresNonFiniteValuesAsNullAndKeepsTheSeriesID(t *testing.T) {
	at := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	notANumber, count, sum := math.NaN(), int64(3), 4.5
	gauge := telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, "gauge"), ProjectID: testProject, EnvironmentID: testEnvironment,
		MetricName: "queue.depth", MetricType: telemetry.MetricTypeGauge, ServiceName: "api", OccurredAt: at, Value: &notANumber}
	histogram := telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, "histogram"), ProjectID: testProject, EnvironmentID: testEnvironment,
		MetricName: "latency", MetricType: telemetry.MetricTypeHistogram, ServiceName: "api", OccurredAt: at, StartedAt: at.Add(-time.Minute),
		HistCount: &count, HistSum: &sum, HistBuckets: []telemetry.HistogramBucket{{UpperBound: 1, Count: 2}, {UpperBound: math.Inf(1), Count: 1}}}
	var rows []storedMetricPoint
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "INSERT INTO tracepilot_metric_points FORMAT JSONEachRow" {
			t.Errorf("unexpected query %q", r.URL.Query().Get("query"))
		}
		rows = decodeRows[storedMetricPoint](t, r)
	})
	if err := store.StoreMetrics(context.Background(), testProject, testEnvironment, []telemetry.MetricDataPoint{gauge, histogram}); err != nil {
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
	if err := store.StoreMetrics(context.Background(), testProject, testEnvironment, []telemetry.MetricDataPoint{bad}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("an unsupported metric type was accepted: %v", err)
	}
}

func TestExpireProjectTelemetryMarksRowsBeforeTheUTCDay(t *testing.T) {
	before := time.Date(2026, 11, 19, 15, 30, 0, 0, time.UTC)
	var mu sync.Mutex
	deletes := map[string]string{}
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query := q.Get("query")
		if strings.HasPrefix(query, "SELECT 1 FROM ") {
			_, _ = w.Write([]byte("1\n")) // every table has an expired row
			return
		}
		if table, found := strings.CutPrefix(query, "DELETE FROM "); found {
			mu.Lock()
			deletes[strings.Fields(table)[0]] = q.Get("param_before")
			mu.Unlock()
		}
	})
	if err := store.ExpireProjectTelemetry(context.Background(), testProject, before); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, table := range []string{"tracepilot_log_records", "tracepilot_spans", "tracepilot_trace_index", "tracepilot_metric_points",
		"tracepilot_span_minutes", "tracepilot_span_minute_marks", "tracepilot_span_minute_done"} {
		if deletes[table] != "2026-11-19 00:00:00" {
			t.Errorf("%s lightweight delete cutoff = %q, want the start of the UTC day", table, deletes[table])
		}
	}
}

func TestDropPartitionsBeforeDropsOnlyFullyExpiredRanges(t *testing.T) {
	var mu sync.Mutex
	var dropped []string
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query := q.Get("query")
		if strings.HasPrefix(query, "SELECT DISTINCT partition_id FROM system.parts") {
			ids := "20261109\n20261116" // weekly partitions start on Monday
			if q.Get("param_table") == "tracepilot_metric_points" {
				ids = "tuple()"
			}
			for _, id := range strings.Split(ids, "\n") {
				fmt.Fprintf(w, "{\"partition_id\":%q}\n", id)
			}
			return
		}
		if rest, found := strings.CutPrefix(query, "ALTER TABLE "); found {
			mu.Lock()
			dropped = append(dropped, rest)
			mu.Unlock()
		}
	})
	before := time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC)
	if err := store.DropPartitionsBefore(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{ // only the week that ended before the cutoff; an unknown ID is kept
		"tracepilot_log_records DROP PARTITION ID '20261109'",
		"tracepilot_spans DROP PARTITION ID '20261109'",
		"tracepilot_trace_index DROP PARTITION ID '20261109'",
		"tracepilot_span_minutes DROP PARTITION ID '20261109'",
		"tracepilot_span_minute_marks DROP PARTITION ID '20261109'",
		"tracepilot_span_minute_done DROP PARTITION ID '20261109'",
	}
	if strings.Join(dropped, "|") != strings.Join(want, "|") {
		t.Fatalf("dropped %q, want %q", dropped, want)
	}
}

func TestPartitionEndCoversEveryGranularity(t *testing.T) {
	for _, test := range []struct {
		granularity config.LogPartition
		id          string
		want        time.Time
	}{
		{config.LogPartitionDaily, "20261119", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)},
		{config.LogPartitionWeekly, "20261116", time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)},
		{"", "20261116", time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)},
		{config.LogPartitionMonthly, "202612", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{config.LogPartitionYearly, "2026", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if got, known := partitionEnd(test.granularity, test.id); !known || !got.Equal(test.want) {
			t.Errorf("partitionEnd(%q, %q) = %s, %t; want %s", test.granularity, test.id, got, known, test.want)
		}
	}
	for _, id := range []string{"all", "2026111", "2026-11-19", "20261119'; DROP TABLE x"} {
		if _, known := partitionEnd(config.LogPartitionDaily, id); known {
			t.Errorf("partition ID %q was accepted", id)
		}
	}
}
