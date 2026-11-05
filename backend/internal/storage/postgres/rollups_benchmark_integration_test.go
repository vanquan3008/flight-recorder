package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// Run against a disposable database with -bench SpanRollup -benchtime=1s.
// Both paths call the same existing store functions over the same 20,000 spans.
func BenchmarkSpanRollupQueriesIntegration(b *testing.B) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		b.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	ctx := context.Background()
	database, err := Open(ctx, config.DatabaseConfig{
		URL: os.Getenv("TRACEPILOT_DATABASE_URL"), MaxConnections: 4,
		MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if err != nil {
		b.Fatal(err)
	}
	stamp := time.Now().UTC()
	project := telemetry.DeterministicID(stamp, "rollup-benchmark", stamp.String())
	scope := config.IngestionConfig{
		ProjectID: project, ProjectSlug: "rollup-" + project[24:], ProjectName: "Rollup benchmark",
		EnvironmentID: telemetry.DeterministicID(stamp, project, "environment"), EnvironmentName: "test",
	}
	if err := database.EnsureIngestionScope(ctx, scope); err != nil {
		database.Close()
		b.Fatal(err)
	}
	b.Cleanup(func() { cleanupIntegrationScope(b, database, project) })
	from := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	for offset := 0; offset < 20_000; offset += 250 {
		events := make([]telemetry.Event, 250)
		for index := range events {
			id := offset + index
			at := from.Add(time.Duration(id%2100) * time.Second)
			event := telemetry.Event{
				ID: telemetry.DeterministicID(at, project, fmt.Sprint(id)), ProjectID: project, Environment: scope.EnvironmentName,
				ServiceName: "api", Signal: telemetry.SignalTrace, Type: telemetry.EventRPC, Status: "OK", SpanKind: 2,
				Timestamp: at, Duration: time.Duration(10+id%1000) * time.Microsecond,
				TraceID: fmt.Sprintf("%032x", id+1), SpanID: fmt.Sprintf("%016x", id+1), Operation: "GET /orders",
				Attributes:       map[string]any{},
				RecordAttributes: map[string]any{"http.route": fmt.Sprintf("/orders/%d", id%4), "http.request.method": "GET", "context": strings.Repeat("payload", 300)},
			}
			if id%4 == 0 {
				event.SpanKind = 3
				event.RecordAttributes = map[string]any{"db.system": "postgresql", "db.name": "orders"}
			}
			events[index] = event
		}
		if err := database.StoreEvents(ctx, scope, events); err != nil {
			b.Fatal(err)
		}
	}
	filter := query.PerformanceFilter{EnvironmentID: scope.EnvironmentID, From: from, To: from.Add(35 * time.Minute), Limit: 50}
	for _, mode := range []string{"raw", "rollup"} {
		if mode == "rollup" {
			if err := database.RefreshSpanRollups(ctx, scope); err != nil {
				b.Fatal(err)
			}
		}
		b.Run(mode+"/performance", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := database.RequestPerformance(ctx, project, filter); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(mode+"/detection", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := database.DetectServiceEvidence(ctx, scope, filter.To); err != nil {
					b.Fatal(err)
				}
				if _, err := database.DetectDependencyEvidence(ctx, scope, filter.To); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
