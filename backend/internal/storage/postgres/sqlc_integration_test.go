package postgres

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

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
