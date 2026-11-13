package postgres

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestRequestPerformanceUsesInboundRequestsAndOneWindowIntegration(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	filter := query.PerformanceFilter{EnvironmentID: scope.EnvironmentID, From: from, To: from.Add(61 * time.Second), Limit: 50}
	var events []telemetry.Event
	add := func(service string, offset time.Duration, latency time.Duration, kind int32, status string, attributes map[string]any) {
		event := flowSpan(scope, service, from.Add(offset), fmt.Sprintf("%016x", len(events)+1), "")
		event.Duration, event.SpanKind, event.Status = latency, kind, status
		event.Operation, event.RecordAttributes = "GET /orders/123", attributes
		events = append(events, event)
	}
	add("api", 0, 100*time.Millisecond, 2, "OK", map[string]any{"http.route": "/orders", "http.request.method": "GET", "http.response.status_code": 200})
	add("api", time.Second, 300*time.Millisecond, 2, "UNSET", map[string]any{"http.route": "/orders", "http.method": "GET", "http.status_code": "500"})
	add("api", 4*time.Second, 500*time.Millisecond, 2, "ERROR", map[string]any{"http.route": "/orders", "http.request.method": "GET", "http.response.status_code": 400})
	add("worker", 60*time.Second, 1100*time.Millisecond, 2, "OK", map[string]any{"rpc.service": "Jobs", "rpc.method": "Run"})
	add("api", 0, 10*time.Second, 3, "ERROR", map[string]any{})
	add("api", 0, 20*time.Second, 1, "ERROR", map[string]any{})
	add("api", -time.Second, time.Second, 2, "ERROR", map[string]any{})
	add("api", 61*time.Second, time.Second, 2, "ERROR", map[string]any{})
	if err := database.StoreEvents(ctx, scope, events); err != nil {
		t.Fatal(err)
	}
	otherEnvironment := scope
	otherEnvironment.EnvironmentID = telemetry.DeterministicID(from, "other-performance-environment", scope.ProjectID)
	otherEnvironment.EnvironmentName = "other"
	if err := database.EnsureIngestionScope(ctx, otherEnvironment); err != nil {
		t.Fatal(err)
	}
	other := flowSpan(otherEnvironment, "api", from, "eeeeeeeeeeeeeeee", "")
	other.Status = "ERROR"
	if err := database.StoreEvents(ctx, otherEnvironment, []telemetry.Event{other}); err != nil {
		t.Fatal(err)
	}
	otherDB, otherProject := newFlowTestDatabase(t)
	other = flowSpan(otherProject, "api", from, "ffffffffffffffff", "")
	if err := otherDB.StoreEvents(ctx, otherProject, []telemetry.Event{other}); err != nil {
		t.Fatal(err)
	}

	result, err := database.RequestPerformance(ctx, scope.ProjectID, filter)
	if err != nil {
		t.Fatal(err)
	}
	stats := result.Summary
	if stats.Requests != 4 || stats.Errors != 2 || stats.ErrorRate != 0.5 || math.Abs(stats.RequestsPerSecond-4.0/61) > 1e-12 || stats.AverageLatencyNanos != int64(500*time.Millisecond) || stats.P95LatencyNanos != int64(1010*time.Millisecond) || stats.P99LatencyNanos != int64(1082*time.Millisecond) {
		t.Fatalf("wrong inbound aggregate: %#v", stats)
	}
	if result.BucketSeconds != 2 || len(result.Series) != 31 || result.Series[0].RequestsPerSecond != 1 || result.Series[1].Requests != 0 || result.Series[30].RequestsPerSecond != 1 || !result.Series[30].To.Equal(filter.To) {
		t.Fatalf("bucket boundaries or idle/partial rates are incorrect: %#v", result.Series)
	}
	if len(result.Services) != 2 || result.Services[0].Service != "api" || len(result.Endpoints) != 2 || result.Endpoints[0].Endpoint != "GET /orders" || result.Endpoints[1].Endpoint != "Jobs/Run" {
		t.Fatalf("wrong breakdown: services=%#v endpoints=%#v", result.Services, result.Endpoints)
	}
	filter.Limit = 1
	limited, err := database.RequestPerformance(ctx, scope.ProjectID, filter)
	if err != nil || len(limited.Services) != 1 || len(limited.Endpoints) != 1 || limited.Summary.Requests != 4 {
		t.Fatalf("breakdown limit truncated totals: %#v, %v", limited, err)
	}
	filter.Endpoint = "GET /orders"
	filtered, err := database.RequestPerformance(ctx, scope.ProjectID, filter)
	if err != nil || filtered.Summary.Requests != 3 || filtered.Summary.P95LatencyNanos != int64(480*time.Millisecond) || math.Abs(filtered.Summary.RequestsPerSecond-3.0/61) > 1e-12 {
		t.Fatalf("endpoint filter: %#v, %v", filtered, err)
	}
	filter.Endpoint, filter.Service = "", "worker"
	filtered, err = database.RequestPerformance(ctx, scope.ProjectID, filter)
	if err != nil || filtered.Summary.Requests != 1 || filtered.Summary.Errors != 0 {
		t.Fatalf("service filter: %#v, %v", filtered, err)
	}
	filter.Service = "missing-service"
	empty, err := database.RequestPerformance(ctx, scope.ProjectID, filter)
	if err != nil || empty.Summary.Requests != 0 || empty.Summary.RequestsPerSecond != 0 || len(empty.Services) != 0 || len(empty.Endpoints) != 0 || len(empty.Series) != 31 {
		t.Fatalf("empty window: %#v, %v", empty, err)
	}
}
