package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// These queries look up related spans (a trace's other spans, a span's parent)
// inside a time bound instead of across every daily partition. The tests pin
// the bound: related spans within an hour are found, spans far outside it are
// not, so the margin stays a deliberate, visible choice.

func TestTraceSummaryCoversSpansWithinAnHourOfTheMatch(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	match := flowSpan(scope, "api", now, "1111111111111111", "")
	near := flowSpan(scope, "worker", now.Add(-30*time.Minute), "2222222222222222", "")
	far := flowSpan(scope, "worker", now.Add(-3*time.Hour), "3333333333333333", "")
	if err := database.StoreEvents(context.Background(), scope, []telemetry.Event{match, near, far}); err != nil {
		t.Fatal(err)
	}
	traces, err := database.ListTraces(context.Background(), scope.ProjectID, query.TraceFilter{
		EnvironmentID: scope.EnvironmentID, Service: "api", From: now.Add(-time.Minute), To: now.Add(time.Minute), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 || traces[0].SpanCount != 2 {
		t.Fatalf("traces = %#v, want one trace of 2 spans (the match and the one 30 minutes earlier; the 3 hour old span is outside the bound)", traces)
	}
}

func TestTopologyFindsParentsThatStartedWithinAnHourBeforeTheWindow(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	nearParent := flowSpan(scope, "gateway", now.Add(-30*time.Minute), "1111111111111111", "")
	nearChild := flowSpan(scope, "worker", now, "2222222222222222", nearParent.SpanID)
	farParent := flowSpan(scope, "batch", now.Add(-3*time.Hour), "3333333333333333", "")
	farParent.TraceID = "22222222222222222222222222222222"
	farChild := flowSpan(scope, "loader", now, "4444444444444444", farParent.SpanID)
	farChild.TraceID = farParent.TraceID
	if err := database.StoreEvents(context.Background(), scope, []telemetry.Event{nearParent, nearChild, farParent, farChild}); err != nil {
		t.Fatal(err)
	}
	topology, err := database.ListTopology(context.Background(), scope.ProjectID, analysis.Filter{EnvironmentID: scope.EnvironmentID, From: now.Add(-time.Minute), To: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	var edges []string
	for _, edge := range topology.Edges {
		edges = append(edges, edge.SourceNodeID+">"+edge.TargetNodeID)
	}
	if len(edges) != 1 || edges[0] != "service:gateway>service:worker" {
		t.Fatalf("edges = %v, want only gateway>worker (the batch>loader parent began 3 hours before the window)", edges)
	}
}
