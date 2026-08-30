package management

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/metrics"
	"github.com/vanquan3008/tracepilot/internal/postgres"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
)

func TestAPIKeyLifecycleIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := postgres.Open(context.Background(), config.DatabaseConfig{
		URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(database.Close)

	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "api-key-lifecycle-project", runID)
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "api-key-" + projectID[24:32], ProjectName: "API Key Lifecycle",
		EnvironmentID: telemetry.DeterministicID(time.Now().UTC(), "api-key-lifecycle-environment", runID), EnvironmentName: "test",
	}
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		t.Fatal(scopeError)
	}
	bootstrapKey := "integration-api-key-admin-" + projectID
	if keyError := database.EnsureBootstrapAPIKey(context.Background(), projectID, bootstrapKey); keyError != nil {
		t.Fatal(keyError)
	}
	server := New(config.Config{
		HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 1024}, Ingestion: ingestion, Database: config.DatabaseConfig{ReadinessTimeout: time.Second}, ShutdownTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), database, nil)

	created := apiKeyRequestResult{}
	response := serveAPIKeyRequest(server, http.MethodPost, "/api/v1/api-keys", bootstrapKey, `{"name":"limited","scopes":["read"]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create API key status=%d body=%s", response.Code, response.Body.String())
	}
	if decodeError := json.Unmarshal(response.Body.Bytes(), &created); decodeError != nil || created.Key == "" || created.APIKey.ID == "" {
		t.Fatalf("decode created API key=%#v error=%v", created, decodeError)
	}
	if strings.Contains(response.Body.String(), "key_hash") {
		t.Fatalf("API key response exposed hash: %s", response.Body.String())
	}

	response = serveAPIKeyRequest(server, http.MethodPost, "/api/v1/api-keys", created.Key, `{"name":"escalation","scopes":["admin"]}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("restricted key could create an admin key: status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAPIKeyRequest(server, http.MethodGet, "/api/v1/logs", created.Key, "")
	if response.Code != http.StatusOK {
		t.Fatalf("restricted read key status=%d body=%s", response.Code, response.Body.String())
	}

	rotated := apiKeyRequestResult{}
	response = serveAPIKeyRequest(server, http.MethodPost, "/api/v1/api-keys/"+created.APIKey.ID+"/rotate", bootstrapKey, "")
	if response.Code != http.StatusOK {
		t.Fatalf("rotate API key status=%d body=%s", response.Code, response.Body.String())
	}
	if decodeError := json.Unmarshal(response.Body.Bytes(), &rotated); decodeError != nil || rotated.Key == "" || rotated.Key == created.Key {
		t.Fatalf("decode rotated API key=%#v error=%v", rotated, decodeError)
	}
	if response = serveAPIKeyRequest(server, http.MethodGet, "/api/v1/logs", created.Key, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("old rotated key status=%d body=%s", response.Code, response.Body.String())
	}
	if response = serveAPIKeyRequest(server, http.MethodGet, "/api/v1/logs", rotated.Key, ""); response.Code != http.StatusOK {
		t.Fatalf("rotated key status=%d body=%s", response.Code, response.Body.String())
	}

	response = serveAPIKeyRequest(server, http.MethodDelete, "/api/v1/api-keys/"+created.APIKey.ID, bootstrapKey, "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("revoke API key status=%d body=%s", response.Code, response.Body.String())
	}
	if response = serveAPIKeyRequest(server, http.MethodGet, "/api/v1/logs", rotated.Key, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key status=%d body=%s", response.Code, response.Body.String())
	}

	keys, listError := database.ListAPIKeys(context.Background(), projectID, postgres.APIKeyFilter{Limit: 10})
	if listError != nil || len(keys) != 2 {
		t.Fatalf("list API keys=%#v error=%v", keys, listError)
	}
	bootstrapPrincipal, authError := database.AuthenticateAPIKey(context.Background(), bootstrapKey)
	if authError != nil {
		t.Fatal(authError)
	}
	response = serveAPIKeyRequest(server, http.MethodDelete, "/api/v1/api-keys/"+bootstrapPrincipal.KeyID, bootstrapKey, "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "LAST_ADMIN_KEY") {
		t.Fatalf("last admin revoke status=%d body=%s", response.Code, response.Body.String())
	}

	outsideProjectID := telemetry.DeterministicID(time.Now().UTC(), "api-key-lifecycle-outside-project", runID)
	outsideIngestion := config.IngestionConfig{
		ProjectID: outsideProjectID, ProjectSlug: "outside-" + outsideProjectID[24:32], ProjectName: "Outside Project",
		EnvironmentID: telemetry.DeterministicID(time.Now().UTC(), "api-key-lifecycle-outside-environment", runID), EnvironmentName: "test",
	}
	if scopeError := database.EnsureIngestionScope(context.Background(), outsideIngestion); scopeError != nil {
		t.Fatal(scopeError)
	}
	outsideKey := "integration-api-key-outside-" + outsideProjectID
	if keyError := database.EnsureBootstrapAPIKey(context.Background(), outsideProjectID, outsideKey); keyError != nil {
		t.Fatal(keyError)
	}
	response = serveAPIKeyRequest(server, http.MethodGet, "/api/v1/api-keys", outsideKey, "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), created.APIKey.ID) {
		t.Fatalf("cross-project API key list status=%d body=%s", response.Code, response.Body.String())
	}
}

type apiKeyRequestResult struct {
	APIKey postgres.APIKey `json:"api_key"`
	Key    string          `json:"key"`
}

func serveAPIKeyRequest(server *Server, method, path, key, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set(apiKeyHeader, key)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	return response
}
