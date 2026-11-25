package traces

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
)

func TestStoreSpansWritesDerivedColumnsAndKeepsTheEventShape(t *testing.T) {
	at := time.Date(2026, 11, 19, 8, 0, 0, 123456789, time.UTC)
	var rows []storedSpan
	store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("query") != "INSERT INTO tracepilot_spans FORMAT JSONEachRow" || q.Get("async_insert") != "1" || q.Get("wait_for_async_insert") != "1" {
			t.Errorf("span insert must wait for the async insert to flush: %v", q)
		}
		rows = clienttest.DecodeRows[storedSpan](t, r)
	})
	if err := store.StoreSpans(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.Event{clienttest.Span(at, "span")}); err != nil {
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
	store := clienttest.Store(t, New, func(http.ResponseWriter, *http.Request) { t.Error("a mismatched batch reached ClickHouse") })
	span := clienttest.Span(time.Now().UTC(), "foreign")
	span.ProjectID = "01900000-0000-7000-8000-000000000099"
	if err := store.StoreSpans(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.Event{span}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("want a permanent content failure, got %v", err)
	}
	log := clienttest.Span(time.Now().UTC(), "log")
	log.Signal, log.Type = telemetry.SignalLog, telemetry.EventLog
	if err := store.StoreSpans(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.Event{log}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("a log was accepted as a span: %v", err)
	}
}
