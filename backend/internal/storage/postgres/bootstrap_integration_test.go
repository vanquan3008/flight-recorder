package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
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

func TestBootstrapKeyLifecycleIntegration(t *testing.T) {
	database := openIntegrationDatabase(t)
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000801",
		ProjectSlug:     "bootstrap-lifecycle",
		ProjectName:     "Bootstrap Lifecycle",
		EnvironmentID:   "01900000-0000-7000-8000-000000000802",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	ctx := context.Background()
	const firstKey = "bootstrap-lifecycle-key-0000000001"
	const rotatedKey = "bootstrap-lifecycle-key-0000000002"

	if ensureError := database.EnsureBootstrapAPIKey(ctx, ingestion.ProjectID, firstKey); ensureError != nil {
		t.Fatal(ensureError)
	}
	if _, authError := database.AuthenticateAPIKey(ctx, firstKey); authError != nil {
		t.Fatalf("fresh bootstrap key rejected: %v", authError)
	}

	if _, revokeError := database.pool.Exec(ctx, "UPDATE api_keys SET revoked_at = now() WHERE project_id = $1 AND name = 'bootstrap'", ingestion.ProjectID); revokeError != nil {
		t.Fatal(revokeError)
	}
	if ensureError := database.EnsureBootstrapAPIKey(ctx, ingestion.ProjectID, firstKey); ensureError != nil {
		t.Fatal(ensureError)
	}
	if _, authError := database.AuthenticateAPIKey(ctx, firstKey); !errors.Is(authError, auth.ErrUnauthorized) {
		t.Fatalf("restart with the same key revived a revoked bootstrap key: %v", authError)
	}

	if ensureError := database.EnsureBootstrapAPIKey(ctx, ingestion.ProjectID, rotatedKey); ensureError != nil {
		t.Fatal(ensureError)
	}
	if _, authError := database.AuthenticateAPIKey(ctx, rotatedKey); authError != nil {
		t.Fatalf("rotated bootstrap key rejected: %v", authError)
	}
	if _, authError := database.AuthenticateAPIKey(ctx, firstKey); !errors.Is(authError, auth.ErrUnauthorized) {
		t.Fatalf("previous bootstrap key still accepted after rotation: %v", authError)
	}
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
			Status:             "OK",
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
	if countError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM spans WHERE project_id = $1", projectID).Scan(&stored); countError != nil {
		t.Fatal(countError)
	}
	if stored != 2 {
		t.Fatalf("spans stored across two environments = %d, want 2", stored)
	}
}
