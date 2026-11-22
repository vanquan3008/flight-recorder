package clickhouse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/db"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestClickHouseRerunAfterLostJournalKeepsLogsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	event := testLog(time.Now().UTC(), "rerun-migration")
	if err := store.StoreLogs(ctx, testProject, testEnvironment, []telemetry.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.execute(ctx, "DROP TABLE tracepilot_schema_migrations", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	state, err := store.MigrationStatus(ctx)
	if err != nil || state.Version != 0 || len(state.Pending) != int(state.Target) {
		t.Fatalf("status without a journal: %+v, %v", state, err)
	}
	if err := store.Ready(ctx); err == nil {
		t.Fatal("an unversioned schema passed readiness")
	}
	for range 2 {
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	row, err := store.GetLog(ctx, testProject, testEnvironment, event.ID)
	if err != nil || row.EventID != event.ID {
		t.Fatalf("rerunning migrations lost a log: %+v, %v", row, err)
	}
	state, err = store.MigrationStatus(ctx)
	if err != nil || state.Version != state.Target || state.Dirty || len(state.Pending) != 0 {
		t.Fatalf("migrated status: %+v, %v", state, err)
	}
	if err := store.Ready(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestClickHouseRefusesNewerSchemaIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	state, err := store.MigrationStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.recordMigration(ctx, state.Target+1, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err == nil {
		t.Fatal("older binary accepted a newer ClickHouse schema")
	}
	if err := store.Ready(ctx); err == nil {
		t.Fatal("newer ClickHouse schema passed readiness")
	}
}

func TestClickHouseReadinessRequiresMaterializedViewsIntegration(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	for _, view := range []struct{ name, migration string }{
		{"tracepilot_trace_index_mv", "000004_tracepilot_trace_index_view.up.sql"},
		{"tracepilot_span_minute_marks_mv", "000008_tracepilot_span_minute_marks_view.up.sql"},
	} {
		if _, err := store.execute(ctx, "DROP VIEW "+view.name, nil, nil, false); err != nil {
			t.Fatal(err)
		}
		if err := store.Ready(ctx); err == nil || !strings.Contains(err.Error(), view.name) {
			t.Fatalf("readiness without %s: %v", view.name, err)
		}
		ddl, err := db.SQL.ReadFile("clickhouse/migrations/" + view.migration)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.applyMigration(ctx, string(ddl)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	span := testSpan(time.Now().UTC(), "after-view-repair")
	storeSpans(t, store, span)
	spans, _, err := store.GetTrace(ctx, testProject, testEnvironment, span.TraceID, query.SpanPage{Limit: 1})
	if err != nil || len(spans) != 1 || spans[0].EventID != span.ID {
		t.Fatalf("trace detail after view repair: %+v, %v", spans, err)
	}
}
