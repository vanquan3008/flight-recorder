package clickhouse

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func integrationStore(t *testing.T, age time.Duration) *Store {
	t.Helper()
	if os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1 and TRACEPILOT_CLICKHOUSE_TEST_URL to a disposable ClickHouse")
	}
	store, err := New(config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"),
		Database: fmt.Sprintf("tracepilot_test_%d", time.Now().UnixNano()), Username: "default",
		RequestTimeout: 10 * time.Second, MaxConnections: 2}, age)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := store.execute(context.Background(), "DROP DATABASE `"+store.cfg.Database+"`", nil, nil, false); err != nil {
			t.Error(err)
		}
		store.Close()
	})
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestWeeklyLogsDeduplicateRetriesPreserveJSONAndPageByNanosecond(t *testing.T) {
	store, ctx := integrationStore(t, 0), context.Background()
	monday := time.Date(2026, 1, 5, 0, 0, 0, 123456789, time.UTC)
	before := testLog(monday.Add(-time.Second), "sunday")
	one, two := testLog(monday, "monday1"), testLog(monday, "monday2")
	one.TraceID, one.SpanID = "11111111111111111111111111111111", "2222222222222222"
	one.ObservedTimestamp = monday.Add(time.Nanosecond)
	one.ServiceName = "api\t' OR 1=1 --\\line\n"
	batch := []telemetry.Event{before, one, two}
	if err := store.StoreLogs(ctx, testProject, testEnvironment, batch); err != nil {
		t.Fatal(err)
	}
	// Different batch boundaries must not change event-level deduplication.
	for _, event := range []telemetry.Event{two, before, one} {
		if err := store.StoreLogs(ctx, testProject, testEnvironment, []telemetry.Event{event}); err != nil {
			t.Fatal(err)
		}
	}
	filter := query.LogFilter{EnvironmentID: testEnvironment, Limit: 1}
	var ids []string
	for {
		rows, err := store.ListLogs(ctx, testProject, filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			break
		}
		// As in the API, the extra row is only a next-page indicator.
		row := rows[0]
		ids = append(ids, row.EventID)
		if len(ids) > 3 {
			t.Fatal("cursor repeated rows")
		}
		filter.Before, filter.CursorID = row.OccurredAt, row.EventID
	}
	if len(ids) != 3 || ids[2] != before.ID || ids[0] == ids[1] {
		t.Fatalf("pagination lost or duplicated logs: %v", ids)
	}
	got, err := store.GetLog(ctx, testProject, testEnvironment, one.ID)
	if err != nil || !got.OccurredAt.Equal(monday) || got.ObservedAt == nil || !got.ObservedAt.Equal(one.ObservedTimestamp) {
		t.Fatalf("timestamp precision lost: %+v, %v", got, err)
	}
	want, _ := eventLog(one)
	wantRecord, _ := want.record()
	if !reflect.DeepEqual(got, wantRecord) {
		t.Fatalf("raw log changed: %+v, want %+v", got, wantRecord)
	}
	filter = query.LogFilter{EnvironmentID: testEnvironment, Service: one.ServiceName, TraceID: one.TraceID, Search: "đặt hàng", Limit: 10}
	if rows, err := store.ListLogs(ctx, testProject, filter); err != nil || len(rows) != 1 || rows[0].EventID != one.ID {
		t.Fatalf("typed filters failed: %+v, %v", rows, err)
	}
	logs, uncorrelated, err := store.LogQuality(ctx, testProject, testEnvironment, "", monday.Add(-time.Minute), monday.Add(time.Minute))
	if err != nil || logs != 3 || uncorrelated != 2 {
		t.Fatalf("counts include retries: %d/%d, %v", logs, uncorrelated, err)
	}
	other := "01900000-0000-7000-8000-000000000099"
	if row, err := store.GetLog(ctx, other, testEnvironment, one.ID); err != nil || row.EventID != "" {
		t.Fatalf("project leak: %+v, %v", row, err)
	}
	if rows, err := store.ListLogs(ctx, testProject, query.LogFilter{EnvironmentID: other, Limit: 10}); err != nil || len(rows) != 0 {
		t.Fatalf("environment leak: %+v, %v", rows, err)
	}
	data, err := store.execute(ctx, "SELECT DISTINCT partition FROM system.parts WHERE database={db:String} AND table='tracepilot_logs' AND active ORDER BY partition FORMAT JSONEachRow", map[string]string{"db": store.cfg.Database}, nil, false)
	if err != nil || string(data) != "{\"partition\":\"2025-12-29\"}\n{\"partition\":\"2026-01-05\"}\n" {
		t.Fatalf("expected Monday UTC week partitions: %s, %v", data, err)
	}
	if err := store.StoreLogRecords(ctx, testProject, testEnvironment, []query.LogRecord{got, got}); err != nil {
		t.Fatal(err)
	}
	// PostgreSQL's timestamp encoding truncates nanos to micros. A pre-cutover
	// exporter retry must still deduplicate against the backfilled row's ID.
	legacy := got
	legacy.OccurredAt = legacy.OccurredAt.Truncate(time.Microsecond)
	if err := store.StoreLogRecords(ctx, testProject, testEnvironment, []query.LogRecord{legacy}); err != nil {
		t.Fatal(err)
	}
	if total, _, err := store.LogQuality(ctx, testProject, testEnvironment, "", monday.Add(-time.Minute), monday.Add(time.Minute)); err != nil || total != 3 {
		t.Fatalf("backfill duplicated rows: %d, %v", total, err)
	}
}

func TestRetentionHidesExpiredLogsBeforeWeeklyPartitionExpires(t *testing.T) {
	store, ctx := integrationStore(t, 24*time.Hour), context.Background()
	now := time.Now().UTC()
	old, recent := testLog(now.Add(-48*time.Hour), "expired"), testLog(now, "recent")
	if err := store.StoreLogs(ctx, testProject, testEnvironment, []telemetry.Event{old, recent}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListLogs(ctx, testProject, query.LogFilter{EnvironmentID: testEnvironment, Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].EventID != recent.ID {
		t.Fatalf("expired log remains visible: %+v, %v", rows, err)
	}
	if row, err := store.GetLog(ctx, testProject, testEnvironment, old.ID); err != nil || row.EventID != "" {
		t.Fatalf("expired detail remains visible: %+v, %v", row, err)
	}
	// Changing retention to forever is an explicit migration, not startup DDL.
	store.maxAge = 0
	if err := store.Ready(ctx); err == nil {
		t.Fatal("startup accepted a stale retention TTL")
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := store.schema(ctx)
	if err != nil || len(state.DDL) == 0 {
		t.Fatal("schema was not preserved", err)
	}
}
