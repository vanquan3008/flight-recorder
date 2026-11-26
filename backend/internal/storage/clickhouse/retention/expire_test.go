package retention

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
)

func TestExpireProjectTelemetryMarksRowsBeforeTheUTCDay(t *testing.T) {
	before := time.Date(2026, 11, 19, 15, 30, 0, 0, time.UTC)
	var mu sync.Mutex
	deletes := map[string]string{}
	store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
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
	if err := store.ExpireProjectTelemetry(context.Background(), telemetrytest.Project, before); err != nil {
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
	store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
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
