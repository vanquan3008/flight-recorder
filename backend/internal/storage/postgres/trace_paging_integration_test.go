package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

const pagingTraceID = "22222222222222222222222222222222"

// seedLargeTrace stores `count` spans of one trace. Several spans share a start
// time, which is where a cursor that looked only at the timestamp would skip or
// repeat rows.
func seedLargeTrace(t *testing.T, database *Database, scope config.IngestionConfig, count int, padding string) []string {
	t.Helper()
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	ids := make([]string, 0, count)
	for start := 0; start < count; start += 250 {
		var batch []telemetry.Event
		for index := start; index < min(start+250, count); index++ {
			event := flowSpan(scope, "api", base.Add(time.Duration(index/3)*time.Millisecond), fmt.Sprintf("%016x", index+1), "")
			event.TraceID = pagingTraceID
			event.RecordAttributes = map[string]any{"padding": padding}
			batch = append(batch, event)
			ids = append(ids, event.ID)
		}
		if err := database.StoreEvents(context.Background(), scope, batch); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

// readAllPages follows the cursor the way a client does and returns the span IDs in order.
func readAllPages(t *testing.T, database *Database, scope config.IngestionConfig, limit int, maxBytes int64) (ids []string, pages int) {
	t.Helper()
	page := query.SpanPage{Limit: limit, MaxBytes: maxBytes}
	for {
		spans, hasMore, err := database.GetTrace(context.Background(), scope.ProjectID, scope.EnvironmentID, pagingTraceID, page)
		if err != nil {
			t.Fatal(err)
		}
		if len(spans) == 0 || len(spans) > limit {
			t.Fatalf("page %d holds %d spans, limit %d", pages, len(spans), limit)
		}
		pages++
		for _, span := range spans {
			ids = append(ids, span.EventID)
		}
		if !hasMore {
			return ids, pages
		}
		last := spans[len(spans)-1]
		page.AfterStartedAt, page.AfterEventID = last.StartedAt, last.EventID
	}
}

func TestTracePagesCoverEverySpanOnceInOrder(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	stored := seedLargeTrace(t, database, scope, 1203, "x")
	got, pages := readAllPages(t, database, scope, 500, 8<<20)
	if pages != 3 || len(got) != len(stored) {
		t.Fatalf("pages=%d spans=%d, want 3 pages covering %d spans", pages, len(got), len(stored))
	}
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("span %s was returned twice", id)
		}
		seen[id] = true
	}
	for _, id := range stored {
		if !seen[id] {
			t.Fatalf("span %s was never returned; paging dropped it", id)
		}
	}
}

func TestTracePageEndsAtByteBudgetButStillReportsMore(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	stored := seedLargeTrace(t, database, scope, 60, string(make([]byte, 0))+fmt.Sprintf("%02000d", 7))
	spans, hasMore, err := database.GetTrace(context.Background(), scope.ProjectID, scope.EnvironmentID, pagingTraceID, query.SpanPage{Limit: 500, MaxBytes: 12 << 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) == 0 || len(spans) >= len(stored) || !hasMore {
		t.Fatalf("got %d of %d spans, more=%t; the byte budget must cut the page short and say more follows", len(spans), len(stored), hasMore)
	}
	all, pages := readAllPages(t, database, scope, 500, 12<<10)
	if len(all) != len(stored) || pages < 2 {
		t.Fatalf("byte-limited paging returned %d of %d spans in %d pages", len(all), len(stored), pages)
	}
}

func TestTracePagingAlwaysAdvancesEvenWhenOneSpanExceedsTheBudget(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	stored := seedLargeTrace(t, database, scope, 5, fmt.Sprintf("%04000d", 1))
	all, pages := readAllPages(t, database, scope, 500, 1) // a budget smaller than any span
	if len(all) != len(stored) || pages != len(stored) {
		t.Fatalf("spans=%d pages=%d, want one span per page and nothing lost", len(all), pages)
	}
}

func TestTracePagesStayInsideTheCallersProject(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	seedLargeTrace(t, database, scope, 3, "x")
	other := scope
	other.ProjectID = "01900000-0000-7000-8000-0000000000ff"
	spans, hasMore, err := database.GetTrace(context.Background(), other.ProjectID, scope.EnvironmentID, pagingTraceID, query.SpanPage{Limit: 500, MaxBytes: 1 << 20})
	if err != nil || len(spans) != 0 || hasMore {
		t.Fatalf("another project read %d spans (more=%t, err=%v)", len(spans), hasMore, err)
	}
}
