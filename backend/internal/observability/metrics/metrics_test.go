package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRuntimeMetricsAreExposed(t *testing.T) {
	registry := New("test", "commit", "time")
	registry.ObserveIncidentDetection(10*time.Millisecond, 1, false)
	registry.ObserveWebhookAttempt("success")
	registry.SetIngestionInflight(2)
	registry.ObserveIngestionShutdown("completed")

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	for _, name := range []string{"tracepilot_incident_detection_runs_total", "tracepilot_webhook_delivery_attempts_total", "tracepilot_ingestion_inflight_events 2", "tracepilot_ingestion_shutdowns_total"} {
		if !strings.Contains(response.Body.String(), name) {
			t.Fatalf("metrics response does not contain %q", name)
		}
	}
}
