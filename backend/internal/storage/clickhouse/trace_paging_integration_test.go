package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

const pagingTraceID = "22222222222222222222222222222222"

func flowSpan(service string, at time.Time, spanID, parent string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, "flow-span", spanID), ProjectID: testProject, EnvironmentID: testEnvironment,
		ServiceName: service, Type: telemetry.EventRPC, Signal: telemetry.SignalTrace, Timestamp: at, Duration: 100 * time.Millisecond,
		Status: "OK", Operation: "work", SpanKind: 2, TraceID: "11111111111111111111111111111111", SpanID: spanID, ParentSpanID: parent}
}

func storeSpans(t *testing.T, store *Store, spans ...telemetry.Event) {
	t.Helper()
	if err := store.StoreSpans(context.Background(), testProject, testEnvironment, spans); err != nil {
		t.Fatal(err)
	}
}

// seedLargeTrace stores count spans of one trace. Spans share start times in
// threes, which is where a cursor on the timestamp alone would skip or repeat rows.
func seedLargeTrace(t *testing.T, store *Store, count int, padding string) []string {
	t.Helper()
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	ids := make([]string, 0, count)
	var spans []telemetry.Event
	for index := range count {
		span := flowSpan("api", base.Add(time.Duration(index/3)*time.Millisecond), fmt.Sprintf("%016x", index+1), "")
		span.TraceID, span.RecordAttributes = pagingTraceID, map[string]any{"padding": padding}
		spans, ids = append(spans, span), append(ids, span.ID)
	}
	storeSpans(t, store, spans...)
	return ids
}

// readAllPages follows the cursor the way a client does and returns the span IDs in order.
func readAllPages(t *testing.T, store *Store, limit int, maxBytes int64) (ids []string, pages int) {
	t.Helper()
	page := query.SpanPage{Limit: limit, MaxBytes: maxBytes}
	for {
		spans, more, err := store.GetTrace(context.Background(), testProject, testEnvironment, pagingTraceID, page)
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
		if !more {
			return ids, pages
		}
		last := spans[len(spans)-1]
		page.AfterStartedAt, page.AfterEventID = last.StartedAt, last.EventID
	}
}

func TestTracePagesCoverEverySpanOnceInOrderIntegration(t *testing.T) {
	store := integrationStore(t)
	stored := seedLargeTrace(t, store, 1203, "x")
	got, pages := readAllPages(t, store, 500, 8<<20)
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
			t.Fatalf("span %s was never returned", id)
		}
	}
}

func TestTracePageByteBudgetIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	stored := seedLargeTrace(t, store, 60, fmt.Sprintf("%02000d", 7))
	spans, more, err := store.GetTrace(ctx, testProject, testEnvironment, pagingTraceID, query.SpanPage{Limit: 500, MaxBytes: 12 << 10})
	if err != nil || len(spans) == 0 || len(spans) >= len(stored) || !more {
		t.Fatalf("got %d of %d spans, more=%t, %v; the byte budget must cut the page and say more follows", len(spans), len(stored), more, err)
	}
	if all, pages := readAllPages(t, store, 500, 12<<10); len(all) != len(stored) || pages < 2 {
		t.Fatalf("byte-limited paging returned %d of %d spans in %d pages", len(all), len(stored), pages)
	}
	if all, pages := readAllPages(t, store, 500, 1); len(all) != len(stored) || pages != len(stored) {
		t.Fatalf("a budget below one span returned %d spans in %d pages, want one span per page", len(all), pages)
	}
	other := "01900000-0000-7000-8000-0000000000ff"
	if spans, more, err := store.GetTrace(ctx, other, testEnvironment, pagingTraceID, query.SpanPage{Limit: 500}); err != nil || len(spans) != 0 || more {
		t.Fatalf("another project read %d spans (more=%t, err=%v)", len(spans), more, err)
	}
}

// pageTraces stores one single-span trace per duration, newest last, and
// returns their IDs newest first: the order a trace list shows them.
func pageTraces(t *testing.T, store *Store, now time.Time, durations []time.Duration) []string {
	t.Helper()
	spans := make([]telemetry.Event, 0, len(durations))
	ids := make([]string, len(durations))
	for index, duration := range durations {
		span := flowSpan("api", now.Add(time.Duration(index-len(durations))*time.Minute), fmt.Sprintf("%016x", index+1), "")
		span.TraceID, span.Duration = fmt.Sprintf("%032x", index+1), duration
		spans = append(spans, span)
		ids[len(durations)-1-index] = span.TraceID
	}
	storeSpans(t, store, spans...)
	return ids
}

func TestTraceListPagesWalkEveryTraceOnceNewestFirstIntegration(t *testing.T) {
	store := integrationStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	want := pageTraces(t, store, now, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond})
	filter := query.TraceFilter{EnvironmentID: testEnvironment, From: now.Add(-time.Hour), To: now, Limit: 2}
	var got []string
	for range len(want) {
		traces, err := store.ListTraces(context.Background(), testProject, filter)
		if err != nil {
			t.Fatal(err)
		}
		page := traces[:min(len(traces), filter.Limit)] // one row past the limit marks a further page
		for _, trace := range page {
			got = append(got, trace.TraceID)
		}
		if len(traces) <= filter.Limit {
			break
		}
		last := page[len(page)-1]
		filter.Before, filter.CursorID = last.CursorAt, last.TraceID
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pages = %v, want %v", got, want)
	}
}

func TestTraceListDurationFilterLooksPastTheFirstPageIntegration(t *testing.T) {
	store := integrationStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	// Only the oldest trace is slow, so a page cut before the duration filter would be empty.
	ids := pageTraces(t, store, now, []time.Duration{3 * time.Second, time.Millisecond, time.Millisecond, time.Millisecond})
	minimum := int64(time.Second)
	traces, err := store.ListTraces(context.Background(), testProject, query.TraceFilter{
		EnvironmentID: testEnvironment, From: now.Add(-time.Hour), To: now, MinDurationNs: &minimum, Limit: 1})
	if err != nil || len(traces) != 1 || traces[0].TraceID != ids[len(ids)-1] {
		t.Fatalf("traces = %+v, %v; want only the slow trace %s", traces, err, ids[len(ids)-1])
	}
}

func TestTraceSummaryWholeTraceWithinAnHourIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	root := flowSpan("gateway", now, "1111111111111111", "")
	child := flowSpan("worker", now.Add(-time.Second), "2222222222222222", root.SpanID)
	child.Status = "ERROR"
	far := flowSpan("worker", now.Add(-3*time.Hour), "3333333333333333", "")
	storeSpans(t, store, root, child, far)
	traces, err := store.ListTraces(ctx, testProject, query.TraceFilter{EnvironmentID: testEnvironment, Service: "worker", Status: "ERROR",
		From: child.Timestamp, To: child.Timestamp, Limit: 50})
	if err != nil || len(traces) != 1 || traces[0].SpanCount != 2 || traces[0].RootService != "gateway" ||
		traces[0].DurationNanos != int64(time.Second+root.Duration) {
		t.Fatalf("traces = %+v, %v; want the whole trace within an hour of the match, rooted at gateway", traces, err)
	}
}

func edgeKinds(topology analysis.Topology) map[string]string {
	edges := map[string]string{}
	for _, edge := range topology.Edges {
		edges[edge.SourceNodeID+">"+edge.TargetNodeID] = edge.Kind
	}
	return edges
}

func TestTopologyParentsIsolatedServicesAndTargetsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	nearParent := flowSpan("gateway", now.Add(-30*time.Minute), "1111111111111111", "")
	nearChild := flowSpan("worker", now, "2222222222222222", nearParent.SpanID)
	farParent := flowSpan("batch", now.Add(-3*time.Hour), "3333333333333333", "")
	farChild := flowSpan("loader", now, "4444444444444444", farParent.SpanID)
	farParent.TraceID, farChild.TraceID = pagingTraceID, pagingTraceID
	lonely := flowSpan("standalone", now, "5555555555555555", "")
	lonely.TraceID = "33333333333333333333333333333333"
	dependency := func(spanID string, kind int32, attributes map[string]any) telemetry.Event {
		span := flowSpan("worker", now, spanID, "")
		span.TraceID, span.SpanKind, span.RecordAttributes = "44444444444444444444444444444444", kind, attributes
		return span
	}
	storeSpans(t, store, nearParent, nearChild, farParent, farChild, lonely,
		dependency("1000000000000001", 3, map[string]any{"db.system": "postgresql", "db.namespace": "orders"}),
		dependency("1000000000000002", 4, map[string]any{"messaging.system": "kafka", "messaging.destination.name": "payments"}),
		dependency("1000000000000003", 3, map[string]any{"peer.service": "billing-api"}),
		dependency("1000000000000004", 2, map[string]any{"http.route": "/orders"}))
	topology, err := store.ListTopology(ctx, testProject, analysis.Filter{EnvironmentID: testEnvironment, From: now, To: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"service:gateway>service:worker": "service", "service:worker>database:orders": "database",
		"service:worker>queue:payments": "queue", "service:worker>external:billing-api": "external"}
	if got := edgeKinds(topology); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("edges = %v, want %v (a parent 3 hours old and a SERVER span add no edge)", got, want)
	}
	found := false
	for _, node := range topology.Nodes {
		found = found || node.ID == "service:standalone"
	}
	if !found {
		t.Fatalf("isolated service missing from nodes: %+v", topology.Nodes)
	}
}
