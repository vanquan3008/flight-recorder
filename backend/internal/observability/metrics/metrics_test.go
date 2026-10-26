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

func TestPoolSaturationIsReadAtScrapeTime(t *testing.T) {
	registry := New("test", "test", "test")
	reading := PoolSnapshot{Acquired: 3, Idle: 1, Max: 4, EmptyAcquires: 7, AcquireWait: 1500 * time.Millisecond}
	registry.WatchPool(func() PoolSnapshot { return reading })
	body := scrape(t, registry)
	for _, want := range []string{
		`tracepilot_db_pool_connections{state="acquired"} 3`, `tracepilot_db_pool_connections{state="max"} 4`,
		`tracepilot_db_pool_empty_acquires_total 7`, `tracepilot_db_pool_acquire_wait_seconds_total 1.5`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scrape is missing %q:\n%s", want, body)
		}
	}
	reading.Acquired = 4
	if !strings.Contains(scrape(t, registry), `tracepilot_db_pool_connections{state="acquired"} 4`) {
		t.Fatal("a later scrape did not see the pool change")
	}
}

func scrape(t *testing.T, registry *Registry) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	registry.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return recorder.Body.String()
}
