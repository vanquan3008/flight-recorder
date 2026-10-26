package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func dependencySpan(scope config.IngestionConfig, at time.Time, spanID string, kind int32, attributes map[string]any, duration time.Duration) telemetry.Event {
	event := flowSpan(scope, "api", at, spanID, "")
	event.SpanKind, event.RecordAttributes, event.Duration = kind, attributes, duration
	return event
}

func TestTopologyNamesDatabaseQueueAndExternalTargets(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	spans := []telemetry.Event{
		dependencySpan(scope, at, "1000000000000001", 3, map[string]any{"db.system": "postgresql", "db.namespace": "orders"}, time.Millisecond),
		dependencySpan(scope, at, "1000000000000002", 4, map[string]any{"messaging.system": "kafka", "messaging.destination.name": "payments"}, time.Millisecond),
		dependencySpan(scope, at, "1000000000000003", 3, map[string]any{"peer.service": "billing-api"}, time.Millisecond),
		dependencySpan(scope, at, "1000000000000004", 2, map[string]any{"http.route": "/orders"}, time.Millisecond),
	}
	if err := database.StoreEvents(ctx, scope, spans); err != nil {
		t.Fatal(err)
	}
	topology, err := database.ListTopology(ctx, scope.ProjectID, analysis.Filter{EnvironmentID: scope.EnvironmentID, From: at.Add(-time.Minute), To: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, edge := range topology.Edges {
		got[edge.TargetNodeID] = edge.Kind
	}
	want := map[string]string{"database:orders": "database", "queue:payments": "queue", "external:billing-api": "external"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("dependency edges = %v, want %v (the SERVER span must not be a dependency)", got, want)
	}
}

func TestDependencyLatencyDetectionUsesSharedTargetName(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	end := time.Now().UTC().Truncate(time.Second)
	orders := map[string]any{"db.system": "postgresql", "db.namespace": "orders"}
	var spans []telemetry.Event
	for index := range 3 { // baseline: fast
		spans = append(spans, dependencySpan(scope, end.Add(-20*time.Minute+time.Duration(index)*time.Second), fmt.Sprintf("20000000000000%02d", index), 3, orders, 10*time.Millisecond))
	}
	for index := range 2 { // current window: much slower
		spans = append(spans, dependencySpan(scope, end.Add(-2*time.Minute+time.Duration(index)*time.Second), fmt.Sprintf("30000000000000%02d", index), 3, orders, 200*time.Millisecond))
	}
	if err := database.StoreEvents(ctx, scope, spans); err != nil {
		t.Fatal(err)
	}
	evidence, err := database.DetectDependencyEvidence(ctx, scope, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].Item.EntityID != "database:orders" || evidence[0].Item.Direction != "increase" {
		t.Fatalf("dependency evidence = %+v, want one latency increase on database:orders", evidence)
	}
}
