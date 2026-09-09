package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestDeleteEventCascadesSignalsAndIncidentLinksIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	projectID := "01900000-0000-7000-8000-000000000601"
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "cascade-integration", ProjectName: "Cascade Integration",
		EnvironmentID: "01900000-0000-7000-8000-000000000602", EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, projectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	spanEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, "cascade", "span"), ProjectID: projectID, Environment: ingestion.EnvironmentName,
		ServiceName: "cascade-service", Type: telemetry.EventHTTPServer, Timestamp: timestamp, Duration: time.Second,
		TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SpanID: "bbbbbbbbbbbbbbbb", Status: "OK", Signal: telemetry.SignalTrace,
		Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
	}
	logEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, "cascade", "log"), ProjectID: projectID, Environment: ingestion.EnvironmentName,
		ServiceName: "cascade-service", Type: telemetry.EventLog, Timestamp: timestamp, Severity: "ERROR", ErrorType: "CascadeError",
		Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
		LogBody: map[string]any{"message": "cascade"},
	}
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{spanEvent, logEvent}); storeError != nil {
		t.Fatal(storeError)
	}
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM events WHERE project_id = $1 AND id IN ($2, $3)", projectID, spanEvent.ID, logEvent.ID); deleteError != nil {
		t.Fatal(deleteError)
	}
	var spans, logs, occurrences, links, incidents int
	if queryError := database.pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM spans WHERE project_id = $1),
			(SELECT count(*) FROM logs WHERE project_id = $1),
			(SELECT count(*) FROM incident_occurrences WHERE project_id = $1),
			(SELECT count(*) FROM incident_events WHERE project_id = $1),
			(SELECT count(*) FROM incidents WHERE project_id = $1)`, projectID).Scan(&spans, &logs, &occurrences, &links, &incidents); queryError != nil {
		t.Fatal(queryError)
	}
	if spans != 0 || logs != 0 || occurrences != 0 || links != 0 || incidents != 1 {
		t.Fatalf("cascade counts spans=%d logs=%d occurrences=%d links=%d incidents=%d", spans, logs, occurrences, links, incidents)
	}
}
