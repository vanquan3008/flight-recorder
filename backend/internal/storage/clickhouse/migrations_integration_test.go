package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestClickHouseAdoptsLegacySchemaWithoutLosingLogsIntegration(t *testing.T) {
	store, ctx := integrationStore(t, 0), context.Background()
	event := testLog(time.Now().UTC(), "legacy-migration")
	if err := store.StoreLogs(ctx, testProject, testEnvironment, []telemetry.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.execute(ctx, "DROP TABLE tracepilot_schema_migrations", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	state, err := store.MigrationStatus(ctx)
	if err != nil || state.Version != 0 || len(state.Pending) != 1 {
		t.Fatalf("legacy status: %+v, %v", state, err)
	}
	if err := store.Ready(ctx); err == nil {
		t.Fatal("unversioned schema was accepted before migration")
	}
	for range 2 {
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	row, err := store.GetLog(ctx, testProject, testEnvironment, event.ID)
	if err != nil || row.EventID != event.ID {
		t.Fatalf("migration lost legacy log: %+v, %v", row, err)
	}
	state, err = store.MigrationStatus(ctx)
	if err != nil || state.Version != state.Target || state.Dirty || len(state.Pending) != 0 {
		t.Fatalf("migrated status: %+v, %v", state, err)
	}
}

func TestClickHouseRefusesNewerSchemaIntegration(t *testing.T) {
	store, ctx := integrationStore(t, 0), context.Background()
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

func TestClickHouseRejectsIncompatibleLegacySchemaIntegration(t *testing.T) {
	store, ctx := integrationStore(t, 0), context.Background()
	for _, sql := range []string{"DROP TABLE tracepilot_schema_migrations", "DROP TABLE tracepilot_logs",
		"CREATE TABLE tracepilot_logs (body String) ENGINE = MergeTree ORDER BY tuple()"} {
		if _, err := store.execute(ctx, sql, nil, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Migrate(ctx); err == nil {
		t.Fatal("incompatible legacy table was adopted")
	}
	state, err := store.MigrationStatus(ctx)
	if err != nil || !state.Dirty {
		t.Fatalf("failed migration did not stay dirty: %+v, %v", state, err)
	}
	if err := store.Migrate(ctx); err == nil {
		t.Fatal("dirty migration was retried automatically")
	}
}
