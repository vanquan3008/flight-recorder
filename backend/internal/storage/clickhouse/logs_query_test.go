package clickhouse

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestListLogsReadsFullRowsOnlyWithinThePageRange(t *testing.T) {
	newest := time.Date(2026, 1, 5, 10, 0, 0, 123456789, time.UTC)
	oldest := newest.Add(-time.Minute)
	var (
		mu      sync.Mutex
		queries []url.Values
	)
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Query())
		first := len(queries) == 1
		mu.Unlock()
		if first {
			fmt.Fprintf(w, "{\"occurred_at\":%q,\"id\":%q}\n{\"occurred_at\":%q,\"id\":%q}\n",
				newest.Format(time.RFC3339Nano), "01900000-0000-7000-8000-0000000000a2",
				oldest.Format(time.RFC3339Nano), "01900000-0000-7000-8000-0000000000a1")
		}
	})
	filter := query.LogFilter{EnvironmentID: testEnvironment, Search: "Timeout", From: newest.Add(-time.Hour), To: newest.Add(time.Hour), Limit: 1}
	if _, err := store.ListLogs(context.Background(), testProject, filter); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("queries = %d, want a key pass and a row pass", len(queries))
	}
	keys, rows := queries[0], queries[1]
	if !strings.HasPrefix(keys.Get("query"), "SELECT occurred_at, toString(event_id) AS id FROM tracepilot_log_records FINAL") || keys.Get("param_limit") != "2" {
		t.Errorf("the key pass must read only the sort key and one extra row: %s limit=%s", keys.Get("query"), keys.Get("param_limit"))
	}
	if !strings.HasPrefix(rows.Get("query"), "SELECT "+logColumns) || rows.Get("param_limit") != "2" ||
		rows.Get("param_from") != parameter(oldest.Format("2006-01-02 15:04:05.999999999")) ||
		rows.Get("param_to") != parameter(newest.Format("2006-01-02 15:04:05.999999999")) {
		t.Errorf("the row pass must be bounded to the page: from=%s to=%s limit=%s", rows.Get("param_from"), rows.Get("param_to"), rows.Get("param_limit"))
	}
	for _, pass := range queries {
		if !strings.Contains(pass.Get("query"), "lowerUTF8(body) LIKE {search:String}") || pass.Get("param_search") != parameter("%timeout%") ||
			!strings.Contains(pass.Get("query"), finalSettings) {
			t.Errorf("both passes must apply the same search and per-partition FINAL: %s", pass.Get("query"))
		}
	}
}

func TestListLogsSkipsTheRowPassForAnEmptyPage(t *testing.T) {
	calls := 0
	store := testStore(t, func(http.ResponseWriter, *http.Request) { calls++ })
	rows, err := store.ListLogs(context.Background(), testProject, query.LogFilter{EnvironmentID: testEnvironment, Limit: 10})
	if err != nil || rows == nil || len(rows) != 0 || calls != 1 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, calls)
	}
}

func TestLogSearchPatternMatchesLiterallyIgnoringCase(t *testing.T) {
	for term, want := range map[string]string{
		"Timeout":    `%timeout%`,
		`say "hi"`:   `%say "hi"%`,
		"line\nnext": "%line\nnext%",
		"50%_off":    `%50\%\_off%`,
		`C:\temp`:    `%c:\\temp%`,
		"Đặt hàng":   `%đặt hàng%`,
	} {
		if got := logSearchPattern(term); got != want {
			t.Errorf("logSearchPattern(%q) = %q, want %q", term, got, want)
		}
	}
}
