package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func openIntegrationDatabase(t *testing.T) *Database {
	t.Helper()
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
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	return database
}

func TestSameSpanInTwoEnvironmentsIntegration(t *testing.T) {
	database := openIntegrationDatabase(t)
	projectID := "01900000-0000-7000-8000-000000000811"
	environments := []config.IngestionConfig{
		{ProjectID: projectID, ProjectSlug: "span-scope", ProjectName: "Span Scope", EnvironmentID: "01900000-0000-7000-8000-000000000812", EnvironmentName: "staging"},
		{ProjectID: projectID, ProjectSlug: "span-scope", ProjectName: "Span Scope", EnvironmentID: "01900000-0000-7000-8000-000000000813", EnvironmentName: "production"},
	}
	t.Cleanup(func() { cleanupIntegrationScope(t, database, projectID) })
	startedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	for _, ingestion := range environments {
		ensureIntegrationScope(t, database, ingestion)
		event := telemetry.Event{
			ID:                 telemetry.DeterministicID(startedAt, ingestion.ProjectID, ingestion.EnvironmentID, "trace-1", "span-1"),
			ProjectID:          ingestion.ProjectID,
			Environment:        ingestion.EnvironmentName,
			ServiceName:        "checkout",
			TraceID:            "0123456789abcdef0123456789abcdef",
			SpanID:             "0123456789abcdef",
			Type:               telemetry.EventCustom,
			Timestamp:          startedAt,
			Duration:           time.Millisecond,
			Status:             "ERROR",
			Operation:          "POST /checkout",
			Attributes:         map[string]any{},
			Signal:             telemetry.SignalTrace,
			ResourceAttributes: map[string]any{},
			ScopeAttributes:    map[string]any{},
			RecordAttributes:   map[string]any{},
			SpanEvents:         []telemetry.SpanEvent{},
			SpanLinks:          []telemetry.SpanLink{},
		}
		if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
			t.Fatalf("store span in %s: %v", ingestion.EnvironmentName, storeError)
		}
	}
	var stored int
	if countError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE project_id = $1", projectID).Scan(&stored); countError != nil {
		t.Fatal(countError)
	}
	if stored != 2 {
		t.Fatalf("envelopes stored across two environments = %d, want 2", stored)
	}
}
