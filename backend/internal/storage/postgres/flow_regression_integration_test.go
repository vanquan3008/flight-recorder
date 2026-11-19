package postgres

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
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

func flowSpan(scope config.IngestionConfig, service string, at time.Time, spanID, parent string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, "flow-span", scope.ProjectID, spanID), ProjectID: scope.ProjectID, Environment: scope.EnvironmentName, ServiceName: service, Type: telemetry.EventRPC, Signal: telemetry.SignalTrace, Timestamp: at, Duration: 100 * time.Millisecond, Status: "OK", Operation: "work", SpanKind: 2, TraceID: "11111111111111111111111111111111", SpanID: spanID, ParentSpanID: parent, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}}
}
