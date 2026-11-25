package traces

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestTracePageStopsBeforeReadingAnOversizedResult(t *testing.T) {
	const pageBudget = 8 << 20
	row := `{"span_events":"[]","span_links":"[]","span_attributes":{"data":"` + strings.Repeat("x", 96<<10) + `"}}` + "\n"
	store := New(clienttest.Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("query"), "tracepilot_trace_index") {
			fmt.Fprintln(w, `{"first_started":"2026-04-01T00:00:00Z","last_ended":"2026-04-01T00:01:00Z"}`)
			return
		}
		for range client.MaxResponseBytes/len(row) + 1 {
			if _, err := io.WriteString(w, row); err != nil {
				return
			}
		}
	}, func(configuration *config.ClickHouseConfig) { configuration.RequestTimeout = 10 * time.Second }))
	page := query.SpanPage{Limit: 500, MaxBytes: pageBudget}
	spans, more, err := store.GetTrace(context.Background(), telemetrytest.Project, telemetrytest.Environment, "22222222222222222222222222222222", page)
	if want := pageBudget / (len(row) - 1); err != nil || len(spans) != want || !more {
		t.Fatalf("got %d spans, more = %t, err = %v, want %d and a next page", len(spans), more, err, want)
	}
}
