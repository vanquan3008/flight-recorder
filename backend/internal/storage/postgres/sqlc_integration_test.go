package postgres

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// BenchmarkStoreEventsIntegration measures incident envelope writes: every
// span in the batch failed, so each one reaches PostgreSQL.
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
	message := strings.Repeat("representative span payload ", 16)
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
				Type:               telemetry.EventHTTPServer,
				Timestamp:          timestamp,
				Duration:           time.Millisecond,
				Status:             "ERROR",
				Operation:          "GET /benchmark",
				TraceID:            fmt.Sprintf("%032x", iteration*batchSize+index+1),
				SpanID:             fmt.Sprintf("%016x", iteration*batchSize+index+1),
				Attributes:         map[string]any{"iteration": iteration, "record": index},
				Signal:             telemetry.SignalTrace,
				ResourceAttributes: map[string]any{"service.name": "benchmark-service"},
				ScopeAttributes:    map[string]any{},
				RecordAttributes:   map[string]any{"source": "benchmark", "message": message},
			}
		}
		if storeError := database.StoreEvents(context.Background(), ingestion, events); storeError != nil {
			b.Fatal(storeError)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(batchSize*b.N)/b.Elapsed().Seconds(), "records/s")
}
