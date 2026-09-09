package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecurityMetricsAreExposed(t *testing.T) {
	registry := New("test", "commit", "time")
	registry.SetSyslogConnections(2)
	registry.ObserveSyslogRecord("accepted", "cef")
	registry.ObserveSecurityDetection(10*time.Millisecond, 1, false)
	registry.ObserveWebhookAttempt("success")
	registry.SetIngestionInflight(2)
	registry.ObserveIngestionShutdown("completed")

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	for _, name := range []string{"tracepilot_security_syslog_connections 2", "tracepilot_security_syslog_records_total", "tracepilot_security_detection_runs_total", "tracepilot_webhook_delivery_attempts_total", "tracepilot_ingestion_inflight_events 2", "tracepilot_ingestion_shutdowns_total"} {
		if !strings.Contains(response.Body.String(), name) {
			t.Fatalf("metrics response does not contain %q", name)
		}
	}
}
