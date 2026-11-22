package clickhouse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestTracePageStopsBeforeReadingAnOversizedResult(t *testing.T) {
	const pageBudget = 8 << 20
	row := `{"span_events":"[]","span_links":"[]","span_attributes":{"data":"` + strings.Repeat("x", 96<<10) + `"}}` + "\n"
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("query"), "tracepilot_trace_index") {
			fmt.Fprintln(w, `{"first_started":"2026-04-01T00:00:00Z","last_ended":"2026-04-01T00:01:00Z"}`)
			return
		}
		for range maxResponseBytes/len(row) + 1 {
			if _, err := io.WriteString(w, row); err != nil {
				return
			}
		}
	})
	store.client.Timeout = 10 * time.Second
	page := query.SpanPage{Limit: 500, MaxBytes: pageBudget}
	spans, more, err := store.GetTrace(context.Background(), testProject, testEnvironment, pagingTraceID, page)
	if want := pageBudget / (len(row) - 1); err != nil || len(spans) != want || !more {
		t.Fatalf("got %d spans, more = %t, err = %v, want %d and a next page", len(spans), more, err, want)
	}
}
