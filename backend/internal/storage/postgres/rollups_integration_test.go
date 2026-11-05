package postgres

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type rollupReadings struct {
	performance  []query.RequestPerformance
	services     [][]sqlcstore.DetectServiceSignalsRow
	dependencies [][]sqlcstore.DetectDependencySignalsRow
}

func readRollupQueries(t *testing.T, database *Database, scope config.IngestionConfig, from time.Time) rollupReadings {
	t.Helper()
	ctx, result := context.Background(), rollupReadings{}
	for _, filter := range []query.PerformanceFilter{
		{From: from, To: from.Add(35 * time.Minute)},
		{From: from.Add(250 * time.Millisecond), To: from.Add(61*time.Second + 250*time.Millisecond)},
		{From: from, To: from.Add(35 * time.Minute), Endpoint: "GET /orders", Service: "api"},
	} {
		filter.EnvironmentID, filter.Limit = scope.EnvironmentID, 1
		performance, err := database.RequestPerformance(ctx, scope.ProjectID, filter)
		if err != nil {
			t.Fatal(err)
		}
		result.performance = append(result.performance, performance)
	}
	queries := sqlcstore.New(database.pool)
	for _, offset := range []time.Duration{0, 500 * time.Millisecond} {
		end := from.Add(35*time.Minute + offset)
		services, err := queries.DetectServiceSignals(ctx, sqlcstore.DetectServiceSignalsParams{
			ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID,
			BaselineStart: end.Add(-35 * time.Minute), CurrentStart: end.Add(-5 * time.Minute), WindowEnd: end,
		})
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(services, func(a, b sqlcstore.DetectServiceSignalsRow) int {
			return cmp.Or(cmp.Compare(a.ServiceName, b.ServiceName), cmp.Compare(a.SpanKind, b.SpanKind))
		})
		dependencies, err := queries.DetectDependencySignals(ctx, sqlcstore.DetectDependencySignalsParams{
			ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID,
			BaselineStart: end.Add(-35 * time.Minute), CurrentStart: end.Add(-5 * time.Minute), WindowEnd: end,
		})
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(dependencies, func(a, b sqlcstore.DetectDependencySignalsRow) int {
			return cmp.Or(cmp.Compare(a.ServiceName, b.ServiceName), cmp.Compare(a.EntityType, b.EntityType), cmp.Compare(a.EntityName, b.EntityName))
		})
		result.services = append(result.services, services)
		result.dependencies = append(result.dependencies, dependencies)
	}
	return result
}

func TestSpanRollupsPreservePerformanceAndDetectionAfterLateRetry(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var events []telemetry.Event
	for index := 0; index < 60; index++ {
		offset := time.Duration(index%20) * 250 * time.Millisecond
		if index >= 20 {
			offset += 30 * time.Minute
		}
		event := flowSpan(scope, "api", from.Add(offset), fmt.Sprintf("%016x", index+1), "")
		event.Duration = time.Duration(index+1) * time.Millisecond
		event.TraceID = fmt.Sprintf("%032x", index+1)
		event.RecordAttributes = map[string]any{"http.route": "/orders", "http.request.method": "GET"}
		if index%3 == 1 {
			event.SpanKind = 5
		}
		if index%3 == 2 {
			event.SpanKind = 3
			event.RecordAttributes = map[string]any{"db.system": "postgresql", "db.name": "orders"}
		}
		if index%7 == 0 {
			event.Status = "ERROR"
		} else if index%5 == 0 {
			event.RecordAttributes["http.response.status_code"] = 500
		}
		events = append(events, event)
	}
	if err := database.StoreEvents(ctx, scope, events); err != nil {
		t.Fatal(err)
	}
	other := scope
	other.EnvironmentID = telemetry.DeterministicID(from, scope.ProjectID, "rollup-other")
	other.EnvironmentName = "other"
	if err := database.EnsureIngestionScope(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := database.StoreEvents(ctx, other, []telemetry.Event{flowSpan(other, "api", from, "ffffffffffffffff", "")}); err != nil {
		t.Fatal(err)
	}
	before := readRollupQueries(t, database, scope, from)
	if before.performance[0].Summary.Requests != 20 || len(before.dependencies[0]) != 1 {
		t.Fatalf("fixture is not exercising both paths: %+v", before)
	}
	for range 2 {
		if err := database.RefreshSpanRollups(ctx, scope); err != nil {
			t.Fatal(err)
		}
	}
	after := readRollupQueries(t, database, scope, from)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rollups changed results:\nbefore=%+v\nafter=%+v", before, after)
	}
	var summaries int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM span_minute_rollups WHERE project_id=$1", scope.ProjectID).Scan(&summaries); err != nil || summaries != 6 {
		t.Fatalf("summary rows=%d, want 6 for 60 spans: %v", summaries, err)
	}
	late := flowSpan(scope, "api", from.Add(31*time.Minute), "eeeeeeeeeeeeeeee", "")
	late.Duration = 3 * time.Second
	for range 2 {
		if err := database.StoreEvents(ctx, scope, []telemetry.Event{events[0], late}); err != nil {
			t.Fatal(err)
		}
	}
	dirty := readRollupQueries(t, database, scope, from)
	if dirty.performance[0].Summary.Requests != 21 {
		t.Fatalf("late or retried data counted incorrectly: %+v", dirty.performance[0])
	}
	if err := database.RefreshSpanRollups(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if clean := readRollupQueries(t, database, scope, from); !reflect.DeepEqual(dirty, clean) {
		t.Fatalf("late data changed across refresh:\ndirty=%+v\nclean=%+v", dirty, clean)
	}
}

func TestSpanRollupsKeepConcurrentIngestDirty(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{flowSpan(scope, "api", at, "1111111111111111", "")}); err != nil {
		t.Fatal(err)
	}
	tx, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := sqlcstore.New(tx)
	minutes, err := queries.LockPendingSpanRollupBuckets(ctx, sqlcstore.LockPendingSpanRollupBucketsParams{ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, RowLimit: 64})
	if err != nil || len(minutes) != 1 {
		t.Fatalf("lock pending minutes: %v, %v", minutes, err)
	}
	done := make(chan error, 1)
	go func() {
		done <- database.StoreEvents(ctx, scope, []telemetry.Event{flowSpan(scope, "api", at.Add(time.Second), "2222222222222222", "")})
	}()
	select {
	case err := <-done:
		t.Fatalf("ingest did not wait for the summary lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := queries.RefreshSpanRollups(ctx, sqlcstore.RefreshSpanRollupsParams{ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, BucketStarts: minutes}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var dirty bool
	if err := database.pool.QueryRow(ctx, "SELECT dirty FROM span_rollup_buckets WHERE project_id=$1", scope.ProjectID).Scan(&dirty); err != nil || !dirty {
		t.Fatalf("concurrent arrival was marked clean: dirty=%t, error=%v", dirty, err)
	}
	before := readRollupQueries(t, database, scope, at)
	if err := database.RefreshSpanRollups(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if after := readRollupQueries(t, database, scope, at); before.performance[0].Summary.Requests != 2 || !reflect.DeepEqual(before, after) {
		t.Fatalf("lost concurrent arrival: before=%+v, after=%+v", before, after)
	}
}

func TestSpanRollupsExpireWithDefaultPartitionRows(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	at := time.Date(2024, 1, 1, 0, 0, 20, 0, time.UTC)
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{
		flowSpan(scope, "api", at, "1111111111111111", ""),
		flowSpan(scope, "api", at.Add(20*time.Second), "2222222222222222", ""),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshSpanRollups(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeleteEventsBefore(ctx, at.Add(10*time.Second), 100); err != nil {
		t.Fatal(err)
	}
	result := readRollupQueries(t, database, scope, at)
	if result.performance[0].Summary.Requests != 1 {
		t.Fatalf("expired data survived in summaries: %+v", result.performance[0])
	}
	var buckets int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM span_rollup_buckets WHERE project_id=$1", scope.ProjectID).Scan(&buckets); err != nil || buckets != 0 {
		t.Fatalf("expired buckets=%d, error=%v", buckets, err)
	}
}

func TestSpanRollupRetentionRollsBackIfSummaryCleanupFails(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	at := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{flowSpan(scope, "api", at, "1111111111111111", "")}); err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshSpanRollups(ctx, scope); err != nil {
		t.Fatal(err)
	}
	_, err := database.pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION test_fail_rollup_retention() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.project_id = '%s'::uuid THEN RAISE EXCEPTION 'forced rollup cleanup failure'; END IF;
			RETURN OLD;
		END $$;
		CREATE TRIGGER test_fail_rollup_retention BEFORE DELETE ON span_rollup_buckets
		FOR EACH ROW EXECUTE FUNCTION test_fail_rollup_retention();`, scope.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := database.pool.Exec(ctx, "DROP TRIGGER test_fail_rollup_retention ON span_rollup_buckets; DROP FUNCTION test_fail_rollup_retention()"); err != nil {
			t.Error(err)
		}
	})
	if deleted, err := database.DeleteEventsBefore(ctx, at.Add(time.Minute), 100); err == nil || deleted != 0 {
		t.Fatalf("failed cleanup returned deleted=%d, error=%v", deleted, err)
	}
	var spans int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM spans WHERE project_id=$1", scope.ProjectID).Scan(&spans); err != nil || spans != 1 {
		t.Fatalf("cleanup removed raw spans without removing summaries: spans=%d, error=%v", spans, err)
	}
}
