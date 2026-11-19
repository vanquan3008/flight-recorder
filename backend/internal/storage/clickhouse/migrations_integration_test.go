package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
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
