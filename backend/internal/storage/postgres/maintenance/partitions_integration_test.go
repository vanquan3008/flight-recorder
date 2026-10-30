package maintenance

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// probeTable is a throwaway partitioned table. The tests drop partitions, and a
// DROP on the shared telemetry tables would take locks that deadlock with other
// packages' tests cleaning up rows, so they use a table of their own. Its name
// follows the "<table>_pYYYYMMDD" convention the retention code recognises.
const probeTable = "retention_probe"

func probeStore(t *testing.T, day time.Time) (*PartitionStore, *pgxpool.Pool, string) {
	t.Helper()
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	ctx := context.Background()
	pool, openError := pgxpool.New(ctx, os.Getenv("TRACEPILOT_DATABASE_URL"))
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(pool.Close)
	partition := partitionName(probeTable, day)
	for _, statement := range []string{
		"DROP TABLE IF EXISTS " + probeTable + " CASCADE",
		"CREATE TABLE " + probeTable + " (occurred_at timestamptz NOT NULL) PARTITION BY RANGE (occurred_at)",
		"CREATE TABLE " + partition + " PARTITION OF " + probeTable + " FOR VALUES FROM ('" + day.Format(partitionBoundLayout) + "') TO ('" + day.AddDate(0, 0, 1).Format(partitionBoundLayout) + "')",
	} {
		if _, execError := pool.Exec(ctx, statement); execError != nil {
			t.Fatal(execError)
		}
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+probeTable+" CASCADE") })
	return New(pool), pool, partition
}

func TestDropExpiredPartitionsDropsEvenWhenCountingFailsIntegration(t *testing.T) {
	day := time.Date(2001, 1, 2, 0, 0, 0, 0, time.UTC)
	store, pool, partition := probeStore(t, day)
	ctx := context.Background()
	exists := func() bool {
		var found bool
		if queryError := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", partition).Scan(&found); queryError != nil {
			t.Fatal(queryError)
		}
		return found
	}
	if !exists() {
		t.Fatalf("%s was not created", partition)
	}

	timedOut := func(context.Context, string) (int64, error) {
		return 0, errors.New("canceling statement due to statement timeout")
	}
	dropped, dropError := store.dropExpiredPartitions(ctx, []string{probeTable}, day.AddDate(0, 0, 1), timedOut)
	if dropError != nil {
		t.Fatalf("a failed count stopped retention: %v", dropError)
	}
	if dropped != 0 {
		t.Fatalf("dropped = %d, want 0 for an uncounted partition", dropped)
	}
	if exists() {
		t.Fatal("the partition stayed because its row count failed; the disk would fill up")
	}
}

func TestDropExpiredPartitionsCountsRowsWhenCountingWorksIntegration(t *testing.T) {
	day := time.Date(2001, 1, 2, 0, 0, 0, 0, time.UTC)
	store, pool, _ := probeStore(t, day)
	ctx := context.Background()
	if _, insertError := pool.Exec(ctx, "INSERT INTO "+probeTable+" SELECT $1::timestamptz + (n || ' seconds')::interval FROM generate_series(1, 5) AS n", day); insertError != nil {
		t.Fatal(insertError)
	}
	dropped, dropError := store.dropExpiredPartitions(ctx, []string{probeTable}, day.AddDate(0, 0, 1), store.countRows)
	if dropError != nil || dropped != 5 {
		t.Fatalf("dropped = %d, err = %v, want the 5 rows that were in the partition", dropped, dropError)
	}
}

func TestDropExpiredPartitionsStopsWhenTheContextIsCancelledIntegration(t *testing.T) {
	day := time.Date(2001, 2, 3, 0, 0, 0, 0, time.UTC)
	store, _, _ := probeStore(t, day)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelling := func(context.Context, string) (int64, error) {
		cancel()
		return 0, context.Canceled
	}
	if _, dropError := store.dropExpiredPartitions(ctx, []string{probeTable}, day.AddDate(0, 0, 1), cancelling); !errors.Is(dropError, context.Canceled) {
		t.Fatalf("shutdown must stop retention, got %v", dropError)
	}
}
