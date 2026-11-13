package postgres

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func newFlowTestDatabase(t *testing.T) (*Database, config.IngestionConfig) {
	t.Helper()
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	url := os.Getenv("TRACEPILOT_DATABASE_URL")
	if url == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, err := OpenWithWebhookEncryptionKey(context.Background(), config.DatabaseConfig{URL: url, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute}, hex.EncodeToString([]byte(strings.Repeat("w", 32))))
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC()
	run := stamp.Format(time.RFC3339Nano) + t.Name()
	project := telemetry.DeterministicID(stamp, "flow-project", run)
	scope := config.IngestionConfig{ProjectID: project, ProjectSlug: "flow-" + project[24:], ProjectName: "Flow regression", EnvironmentID: telemetry.DeterministicID(stamp, "flow-environment", run), EnvironmentName: "test"}
	if err := database.EnsureIngestionScope(context.Background(), scope); err != nil {
		database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := database.pool.Exec(context.Background(), "DELETE FROM webhooks WHERE project_id=$1", project); err != nil {
			t.Error(err)
		}
		cleanupIntegrationScope(t, database, project)
	})
	return database, scope
}

func TestErrorSpanIngestCommitsIncidentEvidenceAndOutbox(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	for _, name := range []string{"first", "second"} {
		if _, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: name, URL: "https://example.com/hook", Events: []string{"incident.opened"}}); err != nil {
			t.Fatal(err)
		}
	}
	later := time.Now().UTC().Truncate(time.Second)
	one := flowSpan(scope, "api", later, "1111111111111111", "")
	one.Status = "ERROR"
	two := flowSpan(scope, "api", later.Add(-time.Minute), "2222222222222222", "")
	two.Status = "ERROR"
	for _, event := range []telemetry.Event{one, two, one} {
		if err := database.StoreEvents(ctx, scope, []telemetry.Event{event}); err != nil {
			t.Fatal(err)
		}
	}
	var incidents, occurrences, deliveries, evidence int
	var first, last time.Time
	if err := database.pool.QueryRow(ctx, `SELECT count(*),min(first_seen_at),max(last_seen_at) FROM incidents WHERE project_id=$1`, scope.ProjectID).Scan(&incidents, &first, &last); err != nil {
		t.Fatal(err)
	}
	for table, target := range map[string]*int{"incident_occurrences": &occurrences, "webhook_deliveries": &deliveries, "incident_evidence": &evidence} {
		if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE project_id=$1", scope.ProjectID).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if incidents != 1 || occurrences != 2 || deliveries != 2 || evidence != 2 || !first.Equal(two.Timestamp) || !last.Equal(one.Timestamp) {
		t.Fatalf("incident=%d occurrence=%d delivery=%d evidence=%d window=%s..%s", incidents, occurrences, deliveries, evidence, first, last)
	}
}

func TestTraceFilterAggregatesWholeTraceAndPrefersActualRoot(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	root := flowSpan(scope, "gateway", now, "1111111111111111", "")
	child := flowSpan(scope, "worker", now.Add(-time.Second), "2222222222222222", root.SpanID)
	child.Status = "ERROR"
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{root, child}); err != nil {
		t.Fatal(err)
	}
	traces, err := database.ListTraces(ctx, scope.ProjectID, query.TraceFilter{EnvironmentID: scope.EnvironmentID, Service: "worker", Status: "ERROR", From: child.Timestamp, To: child.Timestamp, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 || traces[0].SpanCount != 2 || traces[0].RootService != "gateway" || traces[0].DurationNanos != int64(time.Second+root.Duration) {
		t.Fatalf("partial/wrong trace: %#v", traces)
	}
}

func TestTopologyKeepsIsolatedServicesAndParentOutsideWindow(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	parent := flowSpan(scope, "gateway", now.Add(-time.Minute), "1111111111111111", "")
	child := flowSpan(scope, "worker", now, "2222222222222222", parent.SpanID)
	lonely := flowSpan(scope, "standalone", now, "3333333333333333", "")
	if err := database.StoreEvents(context.Background(), scope, []telemetry.Event{parent, child, lonely}); err != nil {
		t.Fatal(err)
	}
	topology, err := database.ListTopology(context.Background(), scope.ProjectID, analysis.Filter{EnvironmentID: scope.EnvironmentID, From: now, To: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range topology.Nodes {
		found = found || node.ID == "service:standalone"
	}
	if !found || len(topology.Edges) != 1 || topology.Edges[0].SourceNodeID != "service:gateway" || topology.Edges[0].TargetNodeID != "service:worker" {
		t.Fatalf("topology=%#v", topology)
	}
}

func flowSpan(scope config.IngestionConfig, service string, at time.Time, spanID, parent string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, "flow-span", scope.ProjectID, spanID), ProjectID: scope.ProjectID, Environment: scope.EnvironmentName, ServiceName: service, Type: telemetry.EventRPC, Signal: telemetry.SignalTrace, Timestamp: at, Duration: 100 * time.Millisecond, Status: "OK", Operation: "work", SpanKind: 2, TraceID: "11111111111111111111111111111111", SpanID: spanID, ParentSpanID: parent, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}}
}
