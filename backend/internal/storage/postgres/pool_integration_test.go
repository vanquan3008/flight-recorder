package postgres

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestStoreEventsIdempotentIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL:                   databaseURL,
		MaxConnections:        4,
		MinConnections:        0,
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}

	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000101",
		ProjectSlug:     "phase2-integration",
		ProjectName:     "Phase 2 Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000102",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	event := telemetry.Event{
		ID:                 telemetry.DeterministicID(timestamp, "integration", "event"),
		ProjectID:          ingestion.ProjectID,
		Environment:        ingestion.EnvironmentName,
		ServiceName:        "integration-service",
		Type:               telemetry.EventLog,
		Timestamp:          timestamp,
		Severity:           "INFO",
		Attributes:         map[string]any{"attempt": "same-event"},
		Signal:             telemetry.SignalLog,
		ResourceAttributes: map[string]any{"service.name": "integration-service"},
		ScopeName:          "integration-test",
		ScopeAttributes:    map[string]any{},
		RecordAttributes:   map[string]any{"attempt": "same-event"},
		SeverityNumber:     9,
		LogBody:            map[string]any{"message": "same event"},
	}
	for range 2 {
		if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
			t.Fatal(storeError)
		}
	}
	var count int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE project_id = $1 AND id = $2", ingestion.ProjectID, event.ID).Scan(&count); queryError != nil {
		t.Fatal(queryError)
	}
	if count != 1 {
		t.Fatalf("idempotent event count = %d", count)
	}
	var severityNumber int
	var body []byte
	if queryError := database.pool.QueryRow(context.Background(), "SELECT severity_number, body FROM logs WHERE project_id = $1 AND event_id = $2", ingestion.ProjectID, event.ID).Scan(&severityNumber, &body); queryError != nil {
		t.Fatal(queryError)
	}
	var storedBody map[string]string
	if decodeError := json.Unmarshal(body, &storedBody); decodeError != nil {
		t.Fatal(decodeError)
	}
	if severityNumber != 9 || storedBody["message"] != "same event" {
		t.Fatalf("stored log = severity=%d body=%s", severityNumber, body)
	}
}

func TestStoreEventsDeduplicatesConcurrentIncidentCandidatesIntegration(t *testing.T) {
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
	t.Cleanup(database.Close)

	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "concurrent-incident-project", runID)
	ingestion := config.IngestionConfig{
		ProjectID:       projectID,
		ProjectSlug:     "concurrent-incident-" + projectID[24:32],
		ProjectName:     "Concurrent Incident Integration",
		EnvironmentID:   telemetry.DeterministicID(time.Now().UTC(), "concurrent-incident-environment", runID),
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })

	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	events := []telemetry.Event{
		{ID: telemetry.DeterministicID(timestamp, "concurrent-incident", "one"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName, ServiceName: "checkout", Type: telemetry.EventException, Timestamp: timestamp, Severity: "ERROR", ErrorType: "timeout", ErrorMessage: "upstream timed out", Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}},
		{ID: telemetry.DeterministicID(timestamp, "concurrent-incident", "two"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName, ServiceName: "checkout", Type: telemetry.EventException, Timestamp: timestamp.Add(time.Second), Severity: "ERROR", ErrorType: "timeout", ErrorMessage: "upstream timed out", Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}},
	}
	var group sync.WaitGroup
	errors := make(chan error, len(events))
	for _, event := range events {
		group.Add(1)
		go func(event telemetry.Event) {
			defer group.Done()
			errors <- database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event})
		}(event)
	}
	group.Wait()
	close(errors)
	for storeError := range errors {
		if storeError != nil {
			t.Fatal(storeError)
		}
	}

	var incidents, occurrences int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM incidents WHERE project_id = $1", ingestion.ProjectID).Scan(&incidents); queryError != nil {
		t.Fatal(queryError)
	}
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM incident_occurrences WHERE project_id = $1", ingestion.ProjectID).Scan(&occurrences); queryError != nil {
		t.Fatal(queryError)
	}
	if incidents != 1 || occurrences != 2 {
		t.Fatalf("incidents=%d occurrences=%d, want 1 and 2", incidents, occurrences)
	}
}

func TestStoreEventsRecoversAfterRolledBackBatchIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL:                   databaseURL,
		MaxConnections:        4,
		MinConnections:        0,
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000201",
		ProjectSlug:     "rollback-integration",
		ProjectName:     "Rollback Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000202",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	badEvent := telemetry.Event{
		ID:          telemetry.DeterministicID(timestamp, "rollback", "bad"),
		ProjectID:   ingestion.ProjectID,
		Environment: ingestion.EnvironmentName,
		ServiceName: "rollback-service",
		Type:        telemetry.EventLog,
		Timestamp:   timestamp,
		Attributes:  map[string]any{},
		Signal:      telemetry.SignalLog,
	}
	badEvent.ID = "not-a-uuid"
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{badEvent}); storeError == nil {
		t.Fatal("expected invalid event ID to roll back the transaction")
	}
	goodEvent := badEvent
	goodEvent.ID = telemetry.DeterministicID(timestamp, "rollback", "good")
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{goodEvent}); storeError != nil {
		t.Fatalf("retry after rollback failed: %v", storeError)
	}
	var count int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE project_id = $1 AND id = $2", ingestion.ProjectID, goodEvent.ID).Scan(&count); queryError != nil {
		t.Fatal(queryError)
	}
	if count != 1 {
		t.Fatalf("retried event count = %d", count)
	}
}

func ensureIntegrationScope(t *testing.T, database *Database, ingestion config.IngestionConfig) {
	t.Helper()
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		t.Fatal(scopeError)
	}
}

func cleanupIntegrationScope(t testing.TB, database *Database, projectID string) {
	t.Helper()
	defer database.Close()
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM incidents WHERE project_id = $1", projectID); deleteError != nil {
		t.Errorf("delete integration incidents: %v", deleteError)
		return
	}
	// Partitioned telemetry has no cascading foreign keys, so each table is cleared explicitly.
	for _, table := range []string{"spans", "logs", "events"} {
		if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM "+table+" WHERE project_id = $1", projectID); deleteError != nil {
			t.Errorf("delete integration %s: %v", table, deleteError)
			return
		}
	}
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM metric_data_points WHERE project_id = $1", projectID); deleteError != nil {
		t.Errorf("delete integration metrics: %v", deleteError)
	}
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM projects WHERE id = $1", projectID); deleteError != nil {
		t.Errorf("delete integration project: %v", deleteError)
	}
}
