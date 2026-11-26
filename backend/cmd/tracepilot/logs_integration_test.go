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

	"github.com/vanquan3008/tracepilot/internal/application/retention"
	"github.com/vanquan3008/tracepilot/internal/application/telemetryquery"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

func TestTelemetryIngestionPreservesIncidentsAndRetry(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" || os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("requires disposable PostgreSQL and ClickHouse integration environments")
	}
	ctx, at := context.Background(), time.Now().UTC().Truncate(time.Second).Add(123456789*time.Nanosecond)
	project := telemetry.DeterministicID(at, "hybrid", t.Name())
	scope := config.IngestionConfig{ProjectID: project, ProjectSlug: "hybrid-" + project[24:], ProjectName: "Hybrid test",
		EnvironmentID: telemetry.DeterministicID(at, project, "env"), EnvironmentName: "test"}
	cfg := config.Config{Ingestion: scope,
		Storage: config.StorageConfig{Retention: config.RetentionConfig{MaxAge: 7 * 24 * time.Hour}},
		Database: config.DatabaseConfig{URL: os.Getenv("TRACEPILOT_DATABASE_URL"), MaxConnections: 4,
			MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute},
		ClickHouse: config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"),
			Database: fmt.Sprintf("tracepilot_test_hybrid_%d", time.Now().UnixNano()), Username: "default", RequestTimeout: 10 * time.Second, MaxConnections: 2}}
	ch, err := clickhouse.New(cfg.ClickHouse)
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
	connections, err := openDatabases(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer connections.lifecycle.Close()
	database, telemetryStore := connections.metadata, connections.telemetry
	policies := retention.NewPolicyService(database, cfg.Storage.Retention.MaxAge, time.Minute, 16, func() time.Time { return at })
	queries := telemetryquery.New(telemetryStore, telemetryStore, telemetryStore, database, policies)
	maintenance := retention.NewTelemetryMaintenance(database, telemetryStore)
	if err := database.EnsureIngestionScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer pool.Close()
		for _, table := range []string{"incidents", "events", "projects"} {
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
	writer := eventWriter{telemetry: telemetryStore, incidents: database}
	write := storeEventsByScope(newScopeCatalog(scope, database), writer.StoreEvents)
	for range 2 {
		if err := write(ctx, []telemetry.Event{info, span, failure}); err != nil {
			t.Fatal(err)
		}
	}
	// PostgreSQL has no signal tables; it keeps only the error log's incident envelope.
	var signalTables, envelopes int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM pg_class WHERE relname IN ('logs', 'spans', 'metric_data_points')),
		(SELECT count(*) FROM events WHERE project_id=$1)`, project).Scan(&signalTables, &envelopes); err != nil || signalTables != 0 || envelopes != 1 {
		t.Fatalf("healthy telemetry reached PG: signal tables=%d events=%d error=%v", signalTables, envelopes, err)
	}
	// A PostgreSQL failure after ClickHouse commits must not ACK; retry repairs it.
	broken, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	broken.Close()
	brokenWriter := eventWriter{telemetry: telemetryStore, incidents: broken}
	retry := failure
	retry.ID = telemetry.DeterministicID(at, project, "retry")
	if err := brokenWriter.StoreEvents(ctx, scope, []telemetry.Event{retry}); err == nil {
		t.Fatal("ACK despite PostgreSQL failure after ClickHouse commit")
	}
	if err := writer.StoreEvents(ctx, scope, []telemetry.Event{retry}); err != nil {
		t.Fatal(err)
	}
	incidents, err := database.ListIncidents(ctx, project, incident.Filter{EnvironmentID: scope.EnvironmentID, Limit: 10})
	if err != nil || len(incidents) != 1 || incidents[0].OccurrenceCount != 2 {
		t.Fatalf("incident retries changed counts: %+v, %v", incidents, err)
	}
	quality, err := queries.TelemetryQuality(ctx, project, analysis.Filter{EnvironmentID: scope.EnvironmentID, From: at.Add(-time.Minute), To: at.Add(time.Minute)})
	if err != nil || quality.Spans != 1 || quality.Logs != 3 || quality.UncorrelatedLogs != 0 {
		t.Fatalf("telemetry quality mixed signals incorrectly: %+v, %v", quality, err)
	}
	// A healthy log routed straight to PostgreSQL writes nothing there.
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{info}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE project_id=$1", project).Scan(&envelopes); err != nil || envelopes != 2 {
		t.Fatalf("envelopes = %d, want the error log and its retry only, %v", envelopes, err)
	}
	if record, err := queries.GetLog(ctx, project, scope.EnvironmentID, info.ID); err != nil || record.EventID != info.ID {
		t.Fatalf("log missing from ClickHouse: %+v, %v", record, err)
	}
	if traces, err := queries.ListTraces(ctx, project, query.TraceFilter{EnvironmentID: scope.EnvironmentID, Limit: 10}); err != nil || len(traces) != 1 {
		t.Fatalf("trace lookup regressed: %+v, %v", traces, err)
	}

	t.Run("saved retention governs reads and deletes", func(t *testing.T) {
		old := info
		old.Timestamp = at.AddDate(0, 0, -10)
		old.ID = telemetry.DeterministicID(old.Timestamp, project, "old")
		if err := telemetryStore.StoreLogs(ctx, project, scope.EnvironmentID, []telemetry.Event{old}); err != nil {
			t.Fatal(err)
		}
		visible := func(want bool) {
			t.Helper()
			row, err := queries.GetLog(ctx, project, scope.EnvironmentID, old.ID)
			if err != nil || (row.EventID == old.ID) != want {
				t.Fatalf("old log visible=%t, want %t, error=%v", row.EventID == old.ID, want, err)
			}
			rows, err := queries.ListLogs(ctx, project, query.LogFilter{EnvironmentID: scope.EnvironmentID, From: old.Timestamp, To: old.Timestamp, Limit: 10})
			if err != nil || (len(rows) == 1) != want {
				t.Fatalf("list returned %d old logs, want visible=%t, error=%v", len(rows), want, err)
			}
		}
		setDays := func(days int32) {
			t.Helper()
			if err := policies.UpdateProjectRetention(ctx, project, &days, auth.Actor{}); err != nil {
				t.Fatal(err)
			}
		}
		visible(false)
		setDays(30)
		visible(true)
		setDays(1)
		if err := maintenance.PruneProject(ctx, project, at, cfg.Storage.Retention.MaxAge); err != nil {
			t.Fatal(err)
		}
		setDays(30)
		visible(false)
	})
}
