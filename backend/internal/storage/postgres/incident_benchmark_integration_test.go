package postgres

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// BenchmarkStoreErrorEventsIntegration measures an error storm: every span in
// the batch fails, spread over a few distinct failures, so each batch opens or
// extends incidents inside the ingest transaction.
func BenchmarkStoreErrorEventsIntegration(b *testing.B) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		b.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		b.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute})
	if openError != nil {
		b.Fatal(openError)
	}
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000511",
		ProjectSlug:     "error-benchmark",
		ProjectName:     "Error Benchmark",
		EnvironmentID:   "01900000-0000-7000-8000-000000000512",
		EnvironmentName: "benchmark",
	}
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		b.Fatal(scopeError)
	}
	b.Cleanup(func() { cleanupIntegrationScope(b, database, ingestion.ProjectID) })

	const batchSize, distinctFailures = 256, 8
	startedAt := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		events := make([]telemetry.Event, batchSize)
		for index := range events {
			timestamp := startedAt.Add(time.Duration(iteration*batchSize+index) * time.Millisecond)
			spanID := fmt.Sprintf("%016x", iteration*batchSize+index+1)
			events[index] = telemetry.Event{
				ID:                 telemetry.DeterministicID(timestamp, "error-benchmark", strconv.Itoa(iteration), strconv.Itoa(index)),
				ProjectID:          ingestion.ProjectID,
				Environment:        ingestion.EnvironmentName,
				ServiceName:        "benchmark-service",
				Type:               telemetry.EventRPC,
				Signal:             telemetry.SignalTrace,
				Timestamp:          timestamp,
				Duration:           time.Millisecond,
				Status:             "ERROR",
				Operation:          "GET /orders/" + strconv.Itoa(index%distinctFailures),
				ErrorMessage:       "order " + strconv.Itoa(index) + " not found",
				TraceID:            hex.EncodeToString([]byte(fmt.Sprintf("%016d", iteration))),
				SpanID:             spanID,
				Attributes:         map[string]any{},
				ResourceAttributes: map[string]any{},
				ScopeAttributes:    map[string]any{},
				RecordAttributes:   map[string]any{},
			}
		}
		if storeError := database.StoreEvents(context.Background(), ingestion, events); storeError != nil {
			b.Fatal(storeError)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(batchSize*b.N)/b.Elapsed().Seconds(), "records/s")
}
