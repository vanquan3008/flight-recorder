package postgres

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestStoreEventsIdempotentIntegration(t *testing.T) {
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
		ProjectID:       "01900000-0000-7000-8000-000000000101",
		ProjectSlug:     "phase2-integration",
		ProjectName:     "Phase 2 Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000102",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	bootstrapKey := "integration-bootstrap-key-00000001"
	if bootstrapError := database.EnsureBootstrapAPIKey(context.Background(), ingestion.ProjectID, bootstrapKey); bootstrapError != nil {
		t.Fatalf("bootstrap API key on fresh scope: %v", bootstrapError)
	}
	principal, authError := database.AuthenticateAPIKey(context.Background(), bootstrapKey)
	if authError != nil || principal.ProjectID != ingestion.ProjectID {
		t.Fatalf("authenticate bootstrap API key: principal=%#v error=%v", principal, authError)
	}
	if slices.Contains(principal.Scopes, "alert") {
		t.Fatal("bootstrap key contains removed alert scope")
	}
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	event := telemetry.Event{
		ID:                 telemetry.DeterministicID(timestamp, "integration", "event"),
		ProjectID:          ingestion.ProjectID,
		Environment:        ingestion.EnvironmentName,
		ServiceName:        "integration-service",
		Type:               telemetry.EventLog,
		Timestamp:          timestamp,
		Severity:           "INFO",
		Attributes:         map[string]any{"attempt": "same-event"},
		Signal:             telemetry.SignalLog,
		ResourceAttributes: map[string]any{"service.name": "integration-service"},
		ScopeName:          "integration-test",
		ScopeAttributes:    map[string]any{},
		RecordAttributes:   map[string]any{"attempt": "same-event"},
		SeverityNumber:     9,
		LogBody:            map[string]any{"message": "same event"},
	}
	for range 2 {
		if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event}); storeError != nil {
			t.Fatal(storeError)
		}
	}
	var count int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE project_id = $1 AND id = $2", ingestion.ProjectID, event.ID).Scan(&count); queryError != nil {
		t.Fatal(queryError)
	}
	if count != 1 {
		t.Fatalf("idempotent event count = %d", count)
	}
	var severityNumber int
	var body []byte
	if queryError := database.pool.QueryRow(context.Background(), "SELECT severity_number, body FROM logs WHERE project_id = $1 AND event_id = $2", ingestion.ProjectID, event.ID).Scan(&severityNumber, &body); queryError != nil {
		t.Fatal(queryError)
	}
	var storedBody map[string]string
	if decodeError := json.Unmarshal(body, &storedBody); decodeError != nil {
		t.Fatal(decodeError)
	}
	if severityNumber != 9 || storedBody["message"] != "same event" {
		t.Fatalf("stored log = severity=%d body=%s", severityNumber, body)
	}
}

func TestStoreEventsDeduplicatesConcurrentIncidentCandidatesIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(database.Close)

	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "concurrent-incident-project", runID)
	ingestion := config.IngestionConfig{
		ProjectID:       projectID,
		ProjectSlug:     "concurrent-incident-" + projectID[24:32],
		ProjectName:     "Concurrent Incident Integration",
		EnvironmentID:   telemetry.DeterministicID(time.Now().UTC(), "concurrent-incident-environment", runID),
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })

	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	events := []telemetry.Event{
		{ID: telemetry.DeterministicID(timestamp, "concurrent-incident", "one"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName, ServiceName: "checkout", Type: telemetry.EventException, Timestamp: timestamp, Severity: "ERROR", ErrorType: "timeout", ErrorMessage: "upstream timed out", Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}},
		{ID: telemetry.DeterministicID(timestamp, "concurrent-incident", "two"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName, ServiceName: "checkout", Type: telemetry.EventException, Timestamp: timestamp.Add(time.Second), Severity: "ERROR", ErrorType: "timeout", ErrorMessage: "upstream timed out", Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}},
	}
	var group sync.WaitGroup
	errors := make(chan error, len(events))
	for _, event := range events {
		group.Add(1)
		go func(event telemetry.Event) {
			defer group.Done()
			errors <- database.StoreEvents(context.Background(), ingestion, []telemetry.Event{event})
		}(event)
	}
	group.Wait()
	close(errors)
	for storeError := range errors {
		if storeError != nil {
			t.Fatal(storeError)
		}
	}

	var incidents, occurrences int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM incidents WHERE project_id = $1", ingestion.ProjectID).Scan(&incidents); queryError != nil {
		t.Fatal(queryError)
	}
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM incident_occurrences WHERE project_id = $1", ingestion.ProjectID).Scan(&occurrences); queryError != nil {
		t.Fatal(queryError)
	}
	if incidents != 1 || occurrences != 2 {
		t.Fatalf("incidents=%d occurrences=%d, want 1 and 2", incidents, occurrences)
	}
}

func TestStoreEventsRecoversAfterRolledBackBatchIntegration(t *testing.T) {
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
		ProjectID:       "01900000-0000-7000-8000-000000000201",
		ProjectSlug:     "rollback-integration",
		ProjectName:     "Rollback Integration",
		EnvironmentID:   "01900000-0000-7000-8000-000000000202",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	badEvent := telemetry.Event{
		ID:          telemetry.DeterministicID(timestamp, "rollback", "bad"),
		ProjectID:   ingestion.ProjectID,
		Environment: ingestion.EnvironmentName,
		ServiceName: "rollback-service",
		Type:        telemetry.EventLog,
		Timestamp:   timestamp,
		Attributes:  map[string]any{},
		Signal:      telemetry.SignalLog,
	}
	badEvent.ID = "not-a-uuid"
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{badEvent}); storeError == nil {
		t.Fatal("expected invalid event ID to roll back the transaction")
	}
	goodEvent := badEvent
	goodEvent.ID = telemetry.DeterministicID(timestamp, "rollback", "good")
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{goodEvent}); storeError != nil {
		t.Fatalf("retry after rollback failed: %v", storeError)
	}
	var count int
	if queryError := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE project_id = $1 AND id = $2", ingestion.ProjectID, goodEvent.ID).Scan(&count); queryError != nil {
		t.Fatal(queryError)
	}
	if count != 1 {
		t.Fatalf("retried event count = %d", count)
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

func TestDeleteEventCascadesSignalsAndIncidentLinksIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	projectID := "01900000-0000-7000-8000-000000000601"
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "cascade-integration", ProjectName: "Cascade Integration",
		EnvironmentID: "01900000-0000-7000-8000-000000000602", EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, projectID) })
	timestamp := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	spanEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, "cascade", "span"), ProjectID: projectID, Environment: ingestion.EnvironmentName,
		ServiceName: "cascade-service", Type: telemetry.EventHTTPServer, Timestamp: timestamp, Duration: time.Second,
		TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SpanID: "bbbbbbbbbbbbbbbb", Status: "OK", Signal: telemetry.SignalTrace,
		Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
	}
	logEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, "cascade", "log"), ProjectID: projectID, Environment: ingestion.EnvironmentName,
		ServiceName: "cascade-service", Type: telemetry.EventLog, Timestamp: timestamp, Severity: "ERROR", ErrorType: "CascadeError",
		Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
		LogBody: map[string]any{"message": "cascade"},
	}
	if storeError := database.StoreEvents(context.Background(), ingestion, []telemetry.Event{spanEvent, logEvent}); storeError != nil {
		t.Fatal(storeError)
	}
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM events WHERE project_id = $1 AND id IN ($2, $3)", projectID, spanEvent.ID, logEvent.ID); deleteError != nil {
		t.Fatal(deleteError)
	}
	var spans, logs, occurrences, links, incidents int
	if queryError := database.pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM spans WHERE project_id = $1),
			(SELECT count(*) FROM logs WHERE project_id = $1),
			(SELECT count(*) FROM incident_occurrences WHERE project_id = $1),
			(SELECT count(*) FROM incident_events WHERE project_id = $1),
			(SELECT count(*) FROM incidents WHERE project_id = $1)`, projectID).Scan(&spans, &logs, &occurrences, &links, &incidents); queryError != nil {
		t.Fatal(queryError)
	}
	if spans != 0 || logs != 0 || occurrences != 0 || links != 0 || incidents != 1 {
		t.Fatalf("cascade counts spans=%d logs=%d occurrences=%d links=%d incidents=%d", spans, logs, occurrences, links, incidents)
	}
}

func TestSQLCServiceAndTraceQueriesIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000701",
		ProjectSlug:     "sqlc-service-queries",
		ProjectName:     "SQLC Service Queries",
		EnvironmentID:   "01900000-0000-7000-8000-000000000702",
		EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, ingestion.ProjectID) })

	timestamp := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	events := []telemetry.Event{
		{
			ID: telemetry.DeterministicID(timestamp, "sqlc-service", "root"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName,
			ServiceName: "checkout", Type: telemetry.EventHTTPServer, Timestamp: timestamp, Duration: time.Second,
			TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SpanID: "bbbbbbbbbbbbbbbb", Status: "ERROR", SpanKind: 2, Signal: telemetry.SignalTrace,
			Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
		},
		{
			ID: telemetry.DeterministicID(timestamp, "sqlc-service", "child"), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName,
			ServiceName: "payment", Type: telemetry.EventHTTPClient, Timestamp: timestamp.Add(time.Millisecond), Duration: 500 * time.Millisecond,
			TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SpanID: "cccccccccccccccc", ParentSpanID: "bbbbbbbbbbbbbbbb", Status: "OK", SpanKind: 3, Signal: telemetry.SignalTrace,
			Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
		},
	}
	if storeError := database.StoreEvents(context.Background(), ingestion, events); storeError != nil {
		t.Fatal(storeError)
	}

	services, queryError := database.ListServices(context.Background(), ingestion.ProjectID, ingestion.EnvironmentID, timestamp.Add(-time.Minute), timestamp.Add(time.Minute))
	if queryError != nil || len(services) != 2 {
		t.Fatalf("ListServices() = %#v, %v", services, queryError)
	}
	if services[0].Name != "checkout" || services[0].RequestCount != 1 || services[0].ErrorCount != 1 {
		t.Fatalf("checkout service = %#v", services[0])
	}

	dependencies, dependencyError := database.ListServiceDependencies(context.Background(), ingestion.ProjectID, ingestion.EnvironmentID, "checkout")
	if dependencyError != nil || len(dependencies) != 1 || dependencies[0].ServiceName != "payment" || dependencies[0].CallCount != 1 {
		t.Fatalf("ListServiceDependencies() = %#v, %v", dependencies, dependencyError)
	}

	spans, traceError := database.GetTrace(context.Background(), ingestion.ProjectID, ingestion.EnvironmentID, events[0].TraceID)
	if traceError != nil || len(spans) != 2 {
		t.Fatalf("GetTrace() = %#v, %v", spans, traceError)
	}
	if spans[0].EventID != events[0].ID || spans[0].ServiceName != "checkout" || spans[1].EventID != events[1].ID || spans[1].ServiceName != "payment" {
		t.Fatalf("GetTrace() spans = %#v", spans)
	}
}

func BenchmarkStoreEventsIntegration(b *testing.B) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		b.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		b.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL:                   databaseURL,
		MaxConnections:        4,
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if openError != nil {
		b.Fatal(openError)
	}
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000501",
		ProjectSlug:     "store-benchmark",
		ProjectName:     "Store Benchmark",
		EnvironmentID:   "01900000-0000-7000-8000-000000000502",
		EnvironmentName: "benchmark",
	}
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		b.Fatal(scopeError)
	}
	b.Cleanup(func() { cleanupIntegrationScope(b, database, ingestion.ProjectID) })

	const batchSize = 256
	message := strings.Repeat("representative log payload ", 16)
	startedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		events := make([]telemetry.Event, batchSize)
		for index := range events {
			timestamp := startedAt.Add(time.Duration(iteration*batchSize+index) * time.Nanosecond)
			events[index] = telemetry.Event{
				ID:                 telemetry.DeterministicID(timestamp, "store-benchmark", strconv.Itoa(iteration), strconv.Itoa(index)),
				ProjectID:          ingestion.ProjectID,
				Environment:        ingestion.EnvironmentName,
				ServiceName:        "benchmark-service",
				Type:               telemetry.EventLog,
				Timestamp:          timestamp,
				Severity:           "INFO",
				Attributes:         map[string]any{"iteration": iteration, "record": index},
				Signal:             telemetry.SignalLog,
				ResourceAttributes: map[string]any{"service.name": "benchmark-service"},
				ScopeAttributes:    map[string]any{},
				RecordAttributes:   map[string]any{"source": "benchmark"},
				LogBody:            map[string]any{"message": message},
			}
		}
		if storeError := database.StoreEvents(context.Background(), ingestion, events); storeError != nil {
			b.Fatal(storeError)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(batchSize*b.N)/b.Elapsed().Seconds(), "records/s")
}

func ensureIntegrationScope(t *testing.T, database *Database, ingestion config.IngestionConfig) {
	t.Helper()
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		t.Fatal(scopeError)
	}
}

func cleanupIntegrationScope(t testing.TB, database *Database, projectID string) {
	t.Helper()
	defer database.Close()
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM incidents WHERE project_id = $1", projectID); deleteError != nil {
		t.Errorf("delete integration incidents: %v", deleteError)
		return
	}
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM events WHERE project_id = $1", projectID); deleteError != nil {
		t.Errorf("delete integration events: %v", deleteError)
		return
	}
	if _, deleteError := database.pool.Exec(context.Background(), "DELETE FROM projects WHERE id = $1", projectID); deleteError != nil {
		t.Errorf("delete integration project: %v", deleteError)
	}
}
