package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/replay"
)

func TestAuthenticatedLogQueryIntegration(t *testing.T) {
	database, ingestion, bootstrapKey := newQueryIntegrationDatabase(t)
	projectID := ingestion.ProjectID
	runID := projectID
	timestamp := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	event := telemetry.Event{
		ID:                 telemetry.DeterministicID(timestamp, "query-integration", projectID),
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
	secondEvent := event
	secondEvent.ID = telemetry.DeterministicID(timestamp.Add(time.Second), "query-integration", projectID)
	secondEvent.Timestamp = timestamp.Add(time.Second)
	spanEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, "query-integration-span", projectID), ProjectID: ingestion.ProjectID,
		Environment: ingestion.EnvironmentName, ServiceName: "query-service", Type: telemetry.EventHTTPServer,
		Timestamp: timestamp, Duration: 500 * time.Millisecond, Operation: "GET /slow", Status: "OK",
		TraceID: "0123456789abcdef0123456789abcdef", SpanID: "0123456789abcdef", Signal: telemetry.SignalTrace,
		Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
	}
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event, secondEvent, spanEvent}); storeError != nil {
		t.Fatal(storeError)
	}
	minimumDuration := int64(250 * time.Millisecond)
	traces, traceError := database.ListTraces(context.Background(), ingestion.ProjectID, query.TraceFilter{EnvironmentID: ingestion.EnvironmentID, MinDurationNs: &minimumDuration, Limit: 10})
	if traceError != nil || len(traces) != 1 || traces[0].TraceID != spanEvent.TraceID {
		t.Fatalf("duration trace query = %#v, error = %v", traces, traceError)
	}
	otherEnvironment := ingestion
	otherEnvironment.EnvironmentID = telemetry.DeterministicID(time.Now().UTC(), "query-integration-other-environment", runID)
	otherEnvironment.EnvironmentName = "other"
	if scopeError := database.EnsureIngestionScope(context.Background(), otherEnvironment); scopeError != nil {
		t.Fatal(scopeError)
	}
	otherEvent := event
	otherEvent.ID = telemetry.DeterministicID(timestamp, "query-integration-other-environment-event", projectID)
	otherEvent.Environment = otherEnvironment.EnvironmentName
	otherEvent.ServiceName = "other-environment-service"
	otherEvent.Severity = "INFO"
	otherEvent.LogBody = map[string]any{"message": "other-environment-only"}
	if storeError := database.StoreEvents(context.Background(), otherEnvironment, []telemetry.Event{otherEvent}); storeError != nil {
		t.Fatal(storeError)
	}

	configuration := config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024},
		Ingestion:       ingestion,
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		Auth:            config.AuthConfig{BootstrapAPIKey: bootstrapKey},
		ShutdownTimeout: time.Second,
	}
	targetServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			responseWriter.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		responseWriter.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(targetServer.Close)
	policy, policyError := replay.NewPolicy([]string{"127.0.0.1"}, true)
	if policyError != nil {
		t.Fatal(policyError)
	}
	executor, executorError := replay.NewExecutor(policy, time.Second, 1024, 1)
	if executorError != nil {
		t.Fatal(executorError)
	}
	replayService := replay.NewService(database, executor)
	server := New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), database, replayService)
	traceRequest := httptest.NewRequest(http.MethodGet, "/api/v1/traces?minDuration=250ms", nil)
	traceRequest.Header.Set(apiKeyHeader, bootstrapKey)
	traceResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(traceResponse, traceRequest)
	if traceResponse.Code != http.StatusOK || !strings.Contains(traceResponse.Body.String(), spanEvent.TraceID) {
		t.Fatalf("trace query status = %d, body = %s", traceResponse.Code, traceResponse.Body.String())
	}
	secureConfiguration := configuration
	secureConfiguration.Production = true
	secureServer := New(secureConfiguration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), database, replayService)
	secureMetrics := httptest.NewRecorder()
	secureServer.httpServer.Handler.ServeHTTP(secureMetrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if secureMetrics.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated production metrics status = %d", secureMetrics.Code)
	}
	secureMetrics = httptest.NewRecorder()
	secureMetricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	secureMetricsRequest.Header.Set(apiKeyHeader, bootstrapKey)
	secureServer.httpServer.Handler.ServeHTTP(secureMetrics, secureMetricsRequest)
	if secureMetrics.Code != http.StatusOK || !strings.Contains(secureMetrics.Body.String(), "tracepilot_build_info") {
		t.Fatalf("authenticated production metrics status = %d body=%s", secureMetrics.Code, secureMetrics.Body.String())
	}

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
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), event.ID) || strings.Contains(response.Body.String(), otherEvent.ID) {
		t.Fatalf("query status = %d, body = %s", response.Code, response.Body.String())
	}

	incidents, incidentError := database.ListIncidents(context.Background(), ingestion.ProjectID, incident.Filter{EnvironmentID: ingestion.EnvironmentID, Limit: 1})
	if incidentError != nil || len(incidents) != 1 {
		t.Fatalf("incidents = %#v, error = %v", incidents, incidentError)
	}
	incidentID := incidents[0].ID
	for _, testCase := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/incidents/not-a-uuid"},
		{method: http.MethodPatch, path: "/api/v1/incidents/not-a-uuid", body: `{"status":"OPEN"}`},
		{method: http.MethodPatch, path: "/api/v1/incidents/" + incidentID, body: `{"status":"CLOSED"}`},
	} {
		request = httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set(apiKeyHeader, bootstrapKey)
		request.Header.Set("Content-Type", "application/json")
		response = httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid incident request %s %s status=%d body=%s", testCase.method, testCase.path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{
		"/api/v1/incidents/" + incidentID + "/occurrences?limit=1",
		"/api/v1/incidents/" + incidentID + "/events?limit=1",
	} {
		request = httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set(apiKeyHeader, bootstrapKey)
		response = httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[`) || !strings.Contains(response.Body.String(), "next_cursor") {
			t.Fatalf("incident evidence %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/incidents/"+incidentID, strings.NewReader(`{"status":"INVESTIGATING"}`))
	request.Header.Set(apiKeyHeader, bootstrapKey)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"INVESTIGATING"`) {
		t.Fatalf("update incident status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+incidentID+"/status-changes", nil)
	request.Header.Set(apiKeyHeader, bootstrapKey)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"previous_status":"OPEN"`) {
		t.Fatalf("incident status history = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/incidents/"+incidentID, strings.NewReader(`{"status":"OPEN"}`))
	request.Header.Set(apiKeyHeader, bootstrapKey)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("invalid incident transition status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/replays", strings.NewReader(fmt.Sprintf(`{"target":%q,"method":"GET","headers":{}}`, targetServer.URL+"/allowed?token=not-stored")))
	request.Header.Set(apiKeyHeader, bootstrapKey)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("allowed request execution status = %d, body = %s", response.Code, response.Body.String())
	}
	executions, executionError := database.ListReplayExecutions(context.Background(), ingestion.ProjectID, ingestion.EnvironmentID, incidentID, replaydomain.ExecutionFilter{Limit: 1})
	if executionError != nil || len(executions) != 1 || executions[0].Status != "succeeded" || strings.Contains(executions[0].Target, "?") {
		t.Fatalf("allowed request execution = %#v, error = %v", executions, executionError)
	}
	for _, path := range []string{
		"/api/v1/incidents/" + incidentID + "/executions",
		"/api/v1/incidents/" + incidentID + "/executions/" + executions[0].ID,
	} {
		request = httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set(apiKeyHeader, bootstrapKey)
		response = httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), executions[0].ID) {
			t.Fatalf("allowed request history %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	slowStarted := make(chan struct{})
	slowTarget := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(slowStarted)
		<-request.Context().Done()
	}))
	t.Cleanup(slowTarget.Close)
	cancelContext, cancelRequest := context.WithCancel(context.Background())
	request = httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/replays", strings.NewReader(fmt.Sprintf(`{"target":%q,"method":"GET","headers":{}}`, slowTarget.URL)))
	request = request.WithContext(cancelContext)
	request.Header.Set(apiKeyHeader, bootstrapKey)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.httpServer.Handler.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-slowStarted:
		cancelRequest()
	case <-time.After(2 * time.Second):
		cancelRequest()
		t.Fatal("replay request did not reach the target")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replay request did not finish after client cancellation")
	}
	if response.Code != http.StatusCreated {
		t.Fatalf("cancelled replay status = %d, body = %s", response.Code, response.Body.String())
	}
	executions, executionError = database.ListReplayExecutions(context.Background(), ingestion.ProjectID, ingestion.EnvironmentID, incidentID, replaydomain.ExecutionFilter{Limit: 10})
	if executionError != nil {
		t.Fatal(executionError)
	}
	failedExecution := false
	for _, execution := range executions {
		if execution.Status == "failed" {
			failedExecution = true
			break
		}
	}
	if !failedExecution {
		t.Fatalf("cancelled replay was not finalized: %#v", executions)
	}

	outsideProject := "01900000-0000-7000-8000-000000000499"
	outside, outsideError := database.GetIncident(context.Background(), outsideProject, ingestion.EnvironmentID, incidentID)
	if outsideError != nil || outside.ID != "" {
		t.Fatalf("cross-project incident read = %#v, error = %v", outside, outsideError)
	}
}
