package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	securityingest "github.com/vanquan3008/tracepilot/internal/security"
)

func TestSecurityDetectionReplayIsIdempotentIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute})
	if openError != nil {
		t.Fatal(openError)
	}
	ingestion := config.IngestionConfig{ProjectID: "01900000-0000-7000-8000-000000000801", ProjectSlug: "detection-replay", ProjectName: "Detection Replay", EnvironmentID: "01900000-0000-7000-8000-000000000802", EnvironmentName: "test"}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	rule, ruleError := database.CreateSecurityRule(context.Background(), ingestion.ProjectID, "", security.Rule{Name: "failed-login", Enabled: true, Severity: "HIGH", Field: "outcome", Operator: "equals", Value: "failure", Threshold: 1, Window: time.Minute})
	if ruleError != nil {
		t.Fatal(ruleError)
	}
	timestamp := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	event := telemetry.Event{ID: telemetry.DeterministicID(timestamp, "detection-replay", "event"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName, ServiceName: "syslog", Type: telemetry.EventLog, Timestamp: timestamp, Signal: telemetry.SignalLog, LogBody: `{"kind":"authentication","outcome":"failure","src":"192.0.2.10"}`, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}}
	event = securityingest.Process([]telemetry.Event{event})[0]
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
		t.Fatal(storeError)
	}
	if count, evaluateError := database.EvaluateSecurityRules(context.Background(), ingestion); evaluateError != nil || count != 1 {
		t.Fatalf("first detection run count=%d error=%v", count, evaluateError)
	}
	if count, evaluateError := database.EvaluateSecurityRules(context.Background(), ingestion); evaluateError != nil || count != 0 {
		t.Fatalf("replayed detection run count=%d error=%v", count, evaluateError)
	}
	var detections, occurrences, occurrenceCount int
	if queryError := database.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM security_detections WHERE project_id=$1 AND rule_id=$2), (SELECT count(*) FROM incident_occurrences WHERE project_id=$1), (SELECT COALESCE(sum(occurrence_count),0) FROM incidents WHERE project_id=$1)`, ingestion.ProjectID, rule.ID).Scan(&detections, &occurrences, &occurrenceCount); queryError != nil {
		t.Fatal(queryError)
	}
	if detections != 1 || occurrences != 1 || occurrenceCount != 1 {
		t.Fatalf("replay duplicated detection/occurrence: detections=%d occurrences=%d occurrence_count=%d", detections, occurrences, occurrenceCount)
	}
}

func TestStoreEventsDeduplicatesIncidentsIntegration(t *testing.T) {
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
		MinConnections:        0,
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000301",
		ProjectSlug:     "incident-integration",
		ProjectName:     "Incident Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000302",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	first := telemetry.Event{
		ID:           telemetry.DeterministicID(timestamp, "incident", "first"),
		ProjectID:    ingestion.ProjectID,
		Environment:  ingestion.EnvironmentName,
		ServiceName:  "checkout",
		TraceID:      "11111111111111111111111111111111",
		SpanID:       "2222222222222222",
		Type:         telemetry.EventException,
		Timestamp:    timestamp,
		HTTPStatus:   500,
		Operation:    "POST /checkout",
		ErrorType:    "PaymentError",
		ErrorMessage: "declined",
		Attributes:   map[string]any{},
	}
	second := first
	second.ID = telemetry.DeterministicID(timestamp.Add(time.Minute), "incident", "second")
	second.Timestamp = timestamp.Add(time.Minute)
	backfill := first
	backfill.ID = telemetry.DeterministicID(timestamp.Add(-24*time.Hour), "incident", "backfill")
	backfill.Timestamp = timestamp.Add(-24 * time.Hour)
	for _, event := range []telemetry.Event{first, second, backfill} {
		if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
			t.Fatal(storeError)
		}
	}
	var incidents, occurrences int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*), COALESCE(max(occurrence_count), 0) FROM incidents WHERE project_id = $1", ingestion.ProjectID).Scan(&incidents, &occurrences); queryError != nil {
		t.Fatal(queryError)
	}
	if incidents != 2 || occurrences != 2 {
		t.Fatalf("incidents=%d occurrences=%d", incidents, occurrences)
	}
}
