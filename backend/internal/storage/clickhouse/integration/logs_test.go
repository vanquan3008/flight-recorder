package integration

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/logs"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestWeeklyLogsDeduplicateRetriesPreserveJSONAndPageByNanosecond(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	monday := time.Date(2026, 1, 5, 0, 0, 0, 123456789, time.UTC)
	before := clienttest.Log(monday.Add(-time.Second), "sunday")
	one, two := clienttest.Log(monday, "monday1"), clienttest.Log(monday, "monday2")
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
	wantRecord, _ := logs.RecordOf(one)
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
	data, err := store.raw.Execute(ctx, "SELECT DISTINCT partition FROM system.parts WHERE database={db:String} AND table='tracepilot_log_records' AND active ORDER BY partition FORMAT JSONEachRow", map[string]string{"db": store.raw.Config().Database}, nil, false)
	if err != nil || string(data) != "{\"partition\":\"2025-12-29\"}\n{\"partition\":\"2026-01-05\"}\n" {
		t.Fatalf("expected Monday UTC week partitions: %s, %v", data, err)
	}
}

func TestLogSearchIgnoresCaseAndMatchesQuotesAndWildcardsLiterally(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	at := time.Now().UTC().Truncate(time.Millisecond)
	match, other := clienttest.Log(at, "search-match"), clienttest.Log(at.Add(time.Millisecond), "search-other")
	match.LogBody = "Payment FAILED: \"card_declined\" 50%"
	other.LogBody = "payment failed: card declined 50x"
	if err := store.StoreLogs(ctx, testProject, testEnvironment, []telemetry.Event{match, other}); err != nil {
		t.Fatal(err)
	}
	filter := query.LogFilter{EnvironmentID: testEnvironment, Search: `failed: "card_declined" 50%`,
		From: at.Add(-time.Minute), To: at.Add(time.Minute), Limit: 10}
	rows, err := store.ListLogs(ctx, testProject, filter)
	if err != nil || len(rows) != 1 || rows[0].EventID != match.ID {
		t.Fatalf("search = %+v, %v; want only the log with the quoted text", rows, err)
	}
}

func TestProjectLogRetentionDeletesOnlyExpiredLogsInThatProject(t *testing.T) {
	store, ctx := integrationStore(t), context.Background()
	now := time.Now().UTC()
	old, recent := clienttest.Log(now.Add(-48*time.Hour), "expired"), clienttest.Log(now, "recent")
	if err := store.StoreLogs(ctx, testProject, testEnvironment, []telemetry.Event{old, recent}); err != nil {
		t.Fatal(err)
	}
	other := clienttest.Log(old.Timestamp, "other-expired")
	other.ProjectID = "01900000-0000-7000-8000-000000000003"
	if err := store.StoreLogs(ctx, other.ProjectID, testEnvironment, []telemetry.Event{other}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.ExpireProjectTelemetry(ctx, testProject, now.Add(-24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListLogs(ctx, testProject, query.LogFilter{EnvironmentID: testEnvironment, Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].EventID != recent.ID {
		t.Fatalf("expired log remains visible: %+v, %v", rows, err)
	}
	if row, err := store.GetLog(ctx, testProject, testEnvironment, old.ID); err != nil || row.EventID != "" {
		t.Fatalf("expired detail remains visible: %+v, %v", row, err)
	}
	if row, err := store.GetLog(ctx, other.ProjectID, testEnvironment, other.ID); err != nil || row.EventID != other.ID {
		t.Fatalf("cleanup affected another project: %+v, %v", row, err)
	}
}
