package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/config"
	"github.com/flight-observability/flight/internal/telemetry"
)

func TestStoreEventsIdempotentIntegration(t *testing.T) {
	if os.Getenv("FLIGHT_INTEGRATION_TEST") != "1" {
		t.Skip("set FLIGHT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("FLIGHT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("FLIGHT_DATABASE_URL is required")
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
	bootstrapKey := "integration-bootstrap-key-00000001"
	if bootstrapError := database.EnsureBootstrapAPIKey(context.Background(), ingestion.ProjectID, bootstrapKey); bootstrapError != nil {
		t.Fatalf("bootstrap API key on fresh scope: %v", bootstrapError)
	}
	if principal, authError := database.AuthenticateAPIKey(context.Background(), bootstrapKey); authError != nil || principal.ProjectID != ingestion.ProjectID {
		t.Fatalf("authenticate bootstrap API key: principal=%#v error=%v", principal, authError)
	}
	t.Cleanup(func() {
		defer database.Close()
		if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM events WHERE project_id = $1", ingestion.ProjectID); deleteError != nil {
			t.Errorf("delete integration events: %v", deleteError)
			return
		}
		if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM projects WHERE id = $1", ingestion.ProjectID); deleteError != nil {
			t.Errorf("delete integration project: %v", deleteError)
		}
	})
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
	if severityNumber != 9 || string(body) != `{"message":"same event"}` {
		t.Fatalf("stored log = severity=%d body=%s", severityNumber, body)
	}
}

func TestStoreEventsRecoversAfterRolledBackBatchIntegration(t *testing.T) {
	if os.Getenv("FLIGHT_INTEGRATION_TEST") != "1" {
		t.Skip("set FLIGHT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("FLIGHT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("FLIGHT_DATABASE_URL is required")
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
	t.Cleanup(func() {
		defer database.Close()
		if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM projects WHERE id = $1", ingestion.ProjectID); deleteError != nil {
			t.Errorf("delete integration project: %v", deleteError)
		}
	})
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	badEvent := telemetry.Event{
		ID:          telemetry.DeterministicID(timestamp, "rollback", "bad"),
		ProjectID:   ingestion.ProjectID,
		Environment: ingestion.EnvironmentName,
		ServiceName: "rollback-service",
		Type:        telemetry.EventLog,
		Timestamp:   timestamp,
		Attributes:  map[string]any{},
		PayloadRef:  "not-a-uuid",
	}
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{badEvent}); storeError == nil {
		t.Fatal("expected bad payload reference to roll back the transaction")
	}
	goodEvent := badEvent
	goodEvent.ID = telemetry.DeterministicID(timestamp, "rollback", "good")
	goodEvent.PayloadRef = ""
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

func TestStoreEventsDeduplicatesIncidentsIntegration(t *testing.T) {
	if os.Getenv("FLIGHT_INTEGRATION_TEST") != "1" {
		t.Skip("set FLIGHT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("FLIGHT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("FLIGHT_DATABASE_URL is required")
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
		ProjectID:       "01900000-0000-7000-8000-000000000301",
		ProjectSlug:     "incident-integration",
		ProjectName:     "Incident Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000302",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() {
		defer database.Close()
		if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM projects WHERE id = $1", ingestion.ProjectID); deleteError != nil {
			t.Errorf("delete integration project: %v", deleteError)
		}
	})
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	first := telemetry.Event{
		ID:           telemetry.DeterministicID(timestamp, "incident", "first"),
		ProjectID:    ingestion.ProjectID,
		Environment:  ingestion.EnvironmentName,
		ServiceName:  "checkout",
		TraceID:      "11111111111111111111111111111111",
		SpanID:       "2222222222222222",
		Type:         telemetry.EventException,
		Timestamp:    timestamp,
		HTTPStatus:   500,
		Operation:    "POST /checkout",
		ErrorType:    "PaymentError",
		ErrorMessage: "declined",
		Attributes:   map[string]any{},
	}
	second := first
	second.ID = telemetry.DeterministicID(timestamp.Add(time.Minute), "incident", "second")
	second.Timestamp = timestamp.Add(time.Minute)
	backfill := first
	backfill.ID = telemetry.DeterministicID(timestamp.Add(-24*time.Hour), "incident", "backfill")
	backfill.Timestamp = timestamp.Add(-24 * time.Hour)
	for _, event := range []telemetry.Event{first, second, backfill} {
		if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
			t.Fatal(storeError)
		}
	}
	var incidents, occurrences int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*), COALESCE(max(occurrence_count), 0) FROM incidents WHERE project_id = $1", ingestion.ProjectID).Scan(&incidents, &occurrences); queryError != nil {
		t.Fatal(queryError)
	}
	if incidents != 2 || occurrences != 2 {
		t.Fatalf("incidents=%d occurrences=%d", incidents, occurrences)
	}
}

func ensureIntegrationScope(t *testing.T, database *Database, ingestion config.IngestionConfig) {
	t.Helper()
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		t.Fatal(scopeError)
	}
}
