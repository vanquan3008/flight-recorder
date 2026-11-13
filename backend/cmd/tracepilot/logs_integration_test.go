package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

func TestHybridLogsPreserveIncidentsRetryAndLegacyBackfill(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" || os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("requires disposable PostgreSQL and ClickHouse integration environments")
	}
	ctx, at := context.Background(), time.Now().UTC().Truncate(time.Second).Add(123456789*time.Nanosecond)
	project := telemetry.DeterministicID(at, "hybrid", t.Name())
	scope := config.IngestionConfig{ProjectID: project, ProjectSlug: "hybrid-" + project[24:], ProjectName: "Hybrid test",
		EnvironmentID: telemetry.DeterministicID(at, project, "env"), EnvironmentName: "test"}
	cfg := config.Config{Ingestion: scope,
		Database: config.DatabaseConfig{URL: os.Getenv("TRACEPILOT_DATABASE_URL"), MaxConnections: 4,
			MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute},
		ClickHouse: config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"),
			Database: fmt.Sprintf("tracepilot_test_hybrid_%d", time.Now().UnixNano()), Username: "default", RequestTimeout: 10 * time.Second, MaxConnections: 2}}
	ch, err := clickhouse.New(cfg.ClickHouse, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if err := ch.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		request, err := http.NewRequest(http.MethodPost, cfg.ClickHouse.URL+"/?query="+url.QueryEscape("DROP DATABASE `"+cfg.ClickHouse.Database+"`"), nil)
		if err != nil {
			t.Error(err)
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			t.Error(err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Error("ClickHouse test cleanup failed", response.StatusCode)
		}
	})
	backend, err := openStorageBackend(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	database := backend.Database
	if err := database.EnsureIngestionScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer pool.Close()
		for _, table := range []string{"incidents", "span_rollup_buckets", "span_minute_rollups", "spans", "logs", "events", "projects"} {
			column := "project_id"
			if table == "projects" {
				column = "id"
			}
			if _, err := pool.Exec(ctx, "DELETE FROM "+table+" WHERE "+column+"=$1", project); err != nil {
				t.Error(err)
			}
		}
	})
	info := telemetry.Event{ID: telemetry.DeterministicID(at, project, "info"), ProjectID: project,
		EnvironmentID: scope.EnvironmentID, Environment: scope.EnvironmentName, ServiceName: "api",
		Timestamp: at, Signal: telemetry.SignalLog, Type: telemetry.EventLog, SeverityNumber: 9,
		TraceID: strings.Repeat("1", 32), SpanID: strings.Repeat("2", 16), LogBody: "normal log",
		Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{}}
	failure := info
	failure.ID, failure.SeverityNumber, failure.ErrorMessage = telemetry.DeterministicID(at, project, "failure"), 17, "database unavailable"
	span := info
	span.ID, span.Signal, span.Type, span.SpanKind = telemetry.DeterministicID(at, project, "span"), telemetry.SignalTrace, telemetry.EventRPC, 2
	span.Status, span.Duration, span.Operation = "OK", time.Millisecond, "GET /orders"
	write := storeEventsByScope(newScopeCatalog(scope, database), backend.StoreEvents)
	for range 2 {
		if err := write(ctx, []telemetry.Event{info, span, failure}); err != nil {
			t.Fatal(err)
		}
	}
	var rawLogs, envelopes int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM logs WHERE project_id=$1), (SELECT count(*) FROM events WHERE project_id=$1)", project).Scan(&rawLogs, &envelopes); err != nil || rawLogs != 0 || envelopes != 2 {
		t.Fatalf("normal log reached PG: logs=%d events=%d error=%v", rawLogs, envelopes, err)
	}
	// A PostgreSQL failure after ClickHouse commits must not ACK; retry repairs it.
	broken, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	broken.Close()
	backend.Database = broken
	retry := failure
	retry.ID = telemetry.DeterministicID(at, project, "retry")
	if err := backend.StoreEvents(ctx, scope, []telemetry.Event{retry}); err == nil {
		t.Fatal("ACK despite PostgreSQL failure after ClickHouse commit")
	}
	backend.Database = database
	if err := backend.StoreEvents(ctx, scope, []telemetry.Event{retry}); err != nil {
		t.Fatal(err)
	}
	incidents, err := backend.ListIncidents(ctx, project, incident.Filter{EnvironmentID: scope.EnvironmentID, Limit: 10})
	if err != nil || len(incidents) != 1 || incidents[0].OccurrenceCount != 2 {
		t.Fatalf("incident retries changed counts: %+v, %v", incidents, err)
	}
	quality, err := backend.TelemetryQuality(ctx, project, analysis.Filter{EnvironmentID: scope.EnvironmentID, From: at.Add(-time.Minute), To: at.Add(time.Minute)})
	if err != nil || quality.Spans != 1 || quality.Logs != 3 || quality.UncorrelatedLogs != 0 {
		t.Fatalf("hybrid quality mixed stores incorrectly: %+v, %v", quality, err)
	}
	legacy := make([]telemetry.Event, 503)
	for index := range legacy {
		legacy[index] = info
		legacy[index].ID = telemetry.DeterministicID(at, project, fmt.Sprint(index))
		legacy[index].Timestamp = at.Add(-time.Hour).Add(time.Duration(index) * time.Nanosecond)
	}
	if err := database.StoreEvents(ctx, scope, legacy); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if count, err := backfillLogs(ctx, database, ch); err != nil || count < len(legacy) {
			t.Fatalf("backfill across a page boundary: count=%d, %v", count, err)
		}
	}
	if err := backend.StoreEvents(ctx, scope, legacy[:1]); err != nil {
		t.Fatal(err)
	}
	logs, _, err := ch.LogQuality(ctx, project, scope.EnvironmentID, "", at.Add(-2*time.Hour), at.Add(time.Minute))
	if err != nil || logs != 506 {
		t.Fatalf("backfill/retry duplicated or lost logs: %d, %v", logs, err)
	}
	if record, err := backend.GetLog(ctx, project, scope.EnvironmentID, legacy[502].ID); err != nil || record.EventID == "" {
		t.Fatalf("last backfill page missing: %+v, %v", record, err)
	}
	if traces, err := backend.ListTraces(ctx, project, query.TraceFilter{EnvironmentID: scope.EnvironmentID, Limit: 10}); err != nil || len(traces) != 1 {
		t.Fatalf("trace lookup regressed: %+v, %v", traces, err)
	}
}
