package integration

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
)

// readFixture stores one trace: an api SERVER root, an orders SERVER child and
// an orders database CLIENT call, plus an orphan span from a second trace.
func readFixture(t *testing.T, store *testStore, at time.Time) (root, child, query_ telemetry.Event) {
	t.Helper()
	span := func(identity, service, spanID, parent string, kind int32, offset, duration time.Duration) telemetry.Event {
		event := clienttest.Span(at.Add(offset), identity)
		event.ServiceName, event.SpanID, event.ParentSpanID, event.SpanKind, event.Duration = service, spanID, parent, kind, duration
		event.TraceID, event.Status, event.Operation, event.RecordAttributes = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "OK", "GET /orders", map[string]any{"http.route": "/orders"}
		return event
	}
	root = span("root", "api", "1111111111111111", "", 2, 0, 40*time.Millisecond)
	child = span("child", "orders", "2222222222222222", root.SpanID, 2, time.Millisecond, 30*time.Millisecond)
	query_ = span("query", "orders", "3333333333333333", child.SpanID, 3, 2*time.Millisecond, 250*time.Microsecond)
	query_.Status, query_.Operation = "ERROR", "SELECT orders"
	query_.RecordAttributes = map[string]any{"db.system": "postgresql", "db.namespace": "orders"}
	orphan := span("orphan", "worker", "4444444444444444", "9999999999999999", 1, 3*time.Millisecond, time.Millisecond)
	orphan.TraceID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for range 2 { // a retried batch must not double any count
		if err := store.StoreSpans(context.Background(), testProject, testEnvironment, []telemetry.Event{root, child, query_, orphan}); err != nil {
			t.Fatal(err)
		}
	}
	return root, child, query_
}

func TestTraceReadsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	root, _, dbCall := readFixture(t, store, at)
	window := query.TraceFilter{EnvironmentID: testEnvironment, From: at.Add(-time.Minute), To: at.Add(time.Minute), Limit: 10}
	traces, err := store.ListTraces(ctx, testProject, window)
	if err != nil || len(traces) != 2 {
		t.Fatalf("traces = %+v, %v", traces, err)
	}
	var main query.TraceSummary
	for _, trace := range traces {
		if trace.TraceID == root.TraceID {
			main = trace
		}
	}
	if main.SpanCount != 3 || main.ErrorCount != 1 || main.RootService != "api" || main.DurationNanos != int64(40*time.Millisecond) {
		t.Fatalf("summary = %+v", main)
	}
	filtered := window
	filtered.Service, filtered.Limit = "orders", 1
	if page, err := store.ListTraces(ctx, testProject, filtered); err != nil || len(page) != 1 || page[0].TraceID != root.TraceID {
		t.Fatalf("service filter = %+v, %v", page, err)
	}
	minimum := int64(35 * time.Millisecond)
	filtered = window
	filtered.MinDurationNs = &minimum
	if page, err := store.ListTraces(ctx, testProject, filtered); err != nil || len(page) != 1 || page[0].TraceID != root.TraceID {
		t.Fatalf("duration filter = %+v, %v", page, err)
	}
	filtered = window
	filtered.Before, filtered.CursorID = traces[0].CursorAt, traces[0].TraceID
	if page, err := store.ListTraces(ctx, testProject, filtered); err != nil || len(page) != 1 || page[0].TraceID == traces[0].TraceID {
		t.Fatalf("cursor page = %+v, %v", page, err)
	}

	spans, more, err := store.GetTrace(ctx, testProject, testEnvironment, root.TraceID, query.SpanPage{Limit: 2})
	if err != nil || len(spans) != 2 || !more || spans[0].SpanID != root.SpanID {
		t.Fatalf("first span page = %+v, more=%t, %v", spans, more, err)
	}
	last := spans[1]
	rest, more, err := store.GetTrace(ctx, testProject, testEnvironment, root.TraceID, query.SpanPage{Limit: 2, AfterStartedAt: last.StartedAt, AfterEventID: last.EventID})
	if err != nil || len(rest) != 1 || more || rest[0].SpanID != dbCall.SpanID || rest[0].DurationNanos != int64(250*time.Microsecond) {
		t.Fatalf("second span page = %+v, more=%t, %v", rest, more, err)
	}
	if missing, more, err := store.GetTrace(ctx, testProject, testEnvironment, "cccccccccccccccccccccccccccccccc", query.SpanPage{Limit: 2}); err != nil || len(missing) != 0 || more {
		t.Fatalf("missing trace = %+v, %t, %v", missing, more, err)
	}
}

func TestServiceAndTopologyReadsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	readFixture(t, store, at)
	services, err := store.ListServices(ctx, testProject, testEnvironment, at.Add(-time.Minute), at.Add(time.Minute))
	if err != nil || len(services) != 3 || services[1].Name != "orders" || services[1].RequestCount != 1 || services[1].ErrorCount != 1 {
		t.Fatalf("services = %+v, %v", services, err)
	}
	dependencies, err := store.ListServiceDependencies(ctx, testProject, testEnvironment, "api", time.Time{})
	if err != nil || len(dependencies) != 1 || dependencies[0].ServiceName != "orders" || dependencies[0].CallCount != 1 {
		t.Fatalf("dependencies = %+v, %v", dependencies, err)
	}
	topology, err := store.ListTopology(ctx, testProject, analysis.Filter{EnvironmentID: testEnvironment, From: at.Add(-time.Minute), To: at.Add(time.Minute)})
	if err != nil || len(topology.Edges) != 2 {
		t.Fatalf("topology = %+v, %v", topology, err)
	}
	edges := map[string]analysis.TopologyEdge{}
	for _, edge := range topology.Edges {
		edges[edge.SourceNodeID+">"+edge.TargetNodeID] = edge
	}
	if edge := edges["service:api>service:orders"]; edge.CallCount != 1 || edge.Kind != "service" {
		t.Fatalf("service edge = %+v", edge)
	}
	if edge := edges["service:orders>database:orders"]; edge.CallCount != 1 || edge.ErrorCount != 1 || edge.Kind != "database" {
		t.Fatalf("database edge = %+v", edge)
	}
	spans, orphans, points, err := store.SignalQuality(ctx, testProject, analysis.Filter{EnvironmentID: testEnvironment, From: at.Add(-time.Minute), To: at.Add(time.Minute)})
	if err != nil || spans != 4 || orphans != 1 || points != 0 {
		t.Fatalf("quality = spans %d orphans %d points %d, %v", spans, orphans, points, err)
	}
}

func TestMetricPointReadsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	value := 3.5
	point := telemetry.MetricDataPoint{ID: telemetry.DeterministicID(at, "read-point"), ProjectID: testProject, EnvironmentID: testEnvironment,
		MetricName: "queue.depth", MetricType: telemetry.MetricTypeGauge, ServiceName: "worker", OccurredAt: at, Value: &value,
		Attributes: map[string]any{"queue": "orders", "shard": int64(2)}}
	if err := store.StoreMetrics(ctx, testProject, testEnvironment, []telemetry.MetricDataPoint{point}); err != nil {
		t.Fatal(err)
	}
	points, err := store.ListMetricPoints(ctx, testProject, analysis.Filter{EnvironmentID: testEnvironment, From: at.Add(-time.Minute), To: at.Add(time.Minute), Metric: "queue.depth", Limit: 10})
	if err != nil || len(points) != 1 || points[0].ID != point.ID || *points[0].Value != value || points[0].Attributes["shard"] != "2" {
		t.Fatalf("metric points = %+v, %v", points, err)
	}
}
