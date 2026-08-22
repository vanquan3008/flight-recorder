package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/config"
	flightmetrics "github.com/flight-observability/flight/internal/metrics"
	"github.com/flight-observability/flight/internal/postgres"
	"github.com/flight-observability/flight/internal/telemetry"
)

func TestAuthenticatedLogQueryIntegration(t *testing.T) {
	if os.Getenv("FLIGHT_INTEGRATION_TEST") != "1" {
		t.Skip("set FLIGHT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("FLIGHT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("FLIGHT_DATABASE_URL is required")
	}
	database, openError := postgres.Open(context.Background(), config.DatabaseConfig{
		URL:                   databaseURL,
		MaxConnections:        4,
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(database.Close)

	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000401",
		ProjectSlug:     "query-integration",
		ProjectName:     "Query Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000402",
		EnvironmentName: "test",
	}
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		t.Fatal(scopeError)
	}
	bootstrapKey := "integration-query-key-000000000001"
	if keyError := database.EnsureBootstrapAPIKey(context.Background(), ingestion.ProjectID, bootstrapKey); keyError != nil {
		t.Fatal(keyError)
	}
	timestamp := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	event := telemetry.Event{
		ID:                 telemetry.DeterministicID(timestamp, "query-integration"),
		ProjectID:          ingestion.ProjectID,
		Environment:        ingestion.EnvironmentName,
		ServiceName:        "query-service",
		Type:               telemetry.EventLog,
		Timestamp:          timestamp,
		Severity:           "ERROR",
		Attributes:         map[string]any{},
		Signal:             telemetry.SignalLog,
		ResourceAttributes: map[string]any{"service.name": "query-service"},
		ScopeAttributes:    map[string]any{},
		RecordAttributes:   map[string]any{},
		LogBody:            map[string]any{"message": "bounded-search-needle"},
	}
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
		t.Fatal(storeError)
	}

	configuration := config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		Auth:            config.AuthConfig{BootstrapAPIKey: bootstrapKey},
		ShutdownTimeout: time.Second,
	}
	server := New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, flightmetrics.New("test", "test", "test"), database, nil)

	unauthorized := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	from := url.QueryEscape(timestamp.Add(-time.Minute).Format(time.RFC3339))
	to := url.QueryEscape(timestamp.Add(time.Minute).Format(time.RFC3339))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=bounded-search-needle&from="+from+"&to="+to, nil)
	request.Header.Set(apiKeyHeader, bootstrapKey)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), event.ID) {
		t.Fatalf("query status = %d, body = %s", response.Code, response.Body.String())
	}
}
