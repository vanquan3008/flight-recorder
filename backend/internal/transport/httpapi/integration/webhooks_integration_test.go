package integration

import (
	"context"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

func TestWebhookManagementAPIIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	key := hex.EncodeToString([]byte(strings.Repeat("a", 32)))
	database, openError := postgres.OpenWithWebhookEncryptionKey(context.Background(), config.DatabaseConfig{URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute}, key)
	if openError != nil {
		t.Fatal(openError)
	}
	defer database.Close()
	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "webhook-http-project", runID)
	ingestion := config.IngestionConfig{ProjectID: projectID, ProjectSlug: "hook-" + projectID[24:32], ProjectName: "Webhook HTTP", EnvironmentID: telemetry.DeterministicID(time.Now().UTC(), "webhook-http-environment", runID), EnvironmentName: "test"}
	if err := database.EnsureIngestionScope(context.Background(), ingestion); err != nil {
		t.Fatal(err)
	}
	var webhookID string
	t.Cleanup(func() {
		if webhookID != "" {
			_, _ = database.DeleteWebhook(context.Background(), projectID, webhookID)
		}
	})
	bootstrap := newIntegrationAdmin(t, database)
	configuration := config.Config{Notifications: config.NotificationsConfig{Enabled: true}, Auth: config.AuthConfig{JWTSecret: integrationJWTSecret}, HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 4096}, Ingestion: ingestion, Database: config.DatabaseConfig{ReadinessTimeout: time.Second}, App: config.AppConfig{ShutdownTimeout: time.Second}}
	server := httpapi.New(configuration, httpapi.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: metrics.New("test", "test", "test"), Readiness: func(context.Context) error { return nil }, Store: postgresBackend{database}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(`{"name":"pager","url":"https://example.com/hook","events":["incident.opened"]}`))
	integrationAuth(request, bootstrap, projectID)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"secret":"whsec_`) {
		t.Fatalf("create webhook = %d %s", response.Code, response.Body.String())
	}
	created := response.Body.String()
	start := strings.Index(created, `"id":"`)
	if start < 0 {
		t.Fatal("create response omitted webhook id")
	}
	webhookID = created[start+6 : start+42]
	request = httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	integrationAuth(request, bootstrap, projectID)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "whsec_") {
		t.Fatalf("list webhook leaked secret or failed: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/rotate", nil)
	integrationAuth(request, bootstrap, projectID)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"secret":"whsec_`) {
		t.Fatalf("rotate webhook = %d %s", response.Code, response.Body.String())
	}
}
