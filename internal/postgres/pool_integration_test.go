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
		ID:          telemetry.DeterministicID(timestamp, "integration", "event"),
		ProjectID:   ingestion.ProjectID,
		Environment: ingestion.EnvironmentName,
		ServiceName: "integration-service",
		Type:        telemetry.EventLog,
		Timestamp:   timestamp,
		Attributes:  map[string]any{"attempt": "same-event"},
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
}
