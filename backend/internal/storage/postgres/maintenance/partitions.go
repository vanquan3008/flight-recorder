package maintenance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Telemetry tables are RANGE-partitioned by day on their event time, so
// retention can drop a whole day instead of deleting its rows. The reasons and
// the constraints this imposes are written at the top of
// db/postgres/migrations/000010_events.up.sql; docs/FLOW.md describes the lifecycle.
//
// Partitions are named <table>_pYYYYMMDD (UTC day). Rows whose day has no
// partition go to <table>_default, so a missing partition never rejects data.

// partitionedTables is every PostgreSQL table partitioned by day; telemetry lives in ClickHouse.
var partitionedTables = []string{"events"}

const (
	// partitionNameDateLayout is the YYYYMMDD suffix of a partition name.
	partitionNameDateLayout = "20060102"
	// partitionBoundLayout renders a UTC midnight as a timestamptz literal.
	partitionBoundLayout = "2006-01-02 15:04:05+00"
	// partitionLockKey serializes partition DDL across replicas so two servers
	// starting together cannot race on the same CREATE TABLE.
	partitionLockKey = 7_203_411_001
)

// dailyPartitionName matches only partitions this package creates. The default
// partition ("<table>_default") deliberately does not match.
var dailyPartitionName = regexp.MustCompile(`^[a-z_]+_p(\d{8})$`)

func partitionName(table string, day time.Time) string {
	return table + "_p" + day.UTC().Format(partitionNameDateLayout)
}

// EnsurePartitions creates the daily partitions that cover `days` days starting
// at the UTC day of `from`, for every partitioned table. It is idempotent and
// safe to call from several replicas at once.
//
// A day that already has rows in a default partition cannot get its own
// partition (PostgreSQL would have to move those rows). That day keeps using
// the default partition and is skipped without an error.
func (store *PartitionStore) EnsurePartitions(ctx context.Context, from time.Time, days int) error {
	if days <= 0 {
		return fmt.Errorf("partition window must be positive, got %d days", days)
	}
	transaction, beginError := store.pool.Begin(ctx)
	if beginError != nil {
		return fmt.Errorf("begin partition maintenance: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if lockError := store.queries.WithTx(transaction).AcquireTransactionLock(ctx, partitionLockKey); lockError != nil {
		return fmt.Errorf("lock partition maintenance: %w", lockError)
	}
	firstDay := from.UTC().Truncate(24 * time.Hour)
	for offset := range days {
		day := firstDay.AddDate(0, 0, offset)
		for _, table := range partitionedTables {
			if createError := createDailyPartition(ctx, transaction, table, day); createError != nil {
				return createError
			}
		}
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return fmt.Errorf("commit partition maintenance: %w", commitError)
	}
	return nil
}

// createDailyPartition creates one partition if it does not exist. Its DDL
// names a table, which cannot be a query parameter, so it is built here rather
// than in db/queries. A creation
// that PostgreSQL rejects (because the default partition already holds rows
// for that day) is rolled back to a savepoint so it cannot abort the other
// days in the same transaction.
func createDailyPartition(ctx context.Context, transaction pgx.Tx, table string, day time.Time) error {
	name := partitionName(table, day)
	// Names are quoted identifiers and bounds come from time.Time, never from
	// request data.
	create := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		identifier(name), identifier(table), day.Format(partitionBoundLayout), day.AddDate(0, 0, 1).Format(partitionBoundLayout))

	if _, savepointError := transaction.Exec(ctx, "SAVEPOINT create_partition"); savepointError != nil {
		return fmt.Errorf("savepoint for %s: %w", name, savepointError)
	}
	if _, createError := transaction.Exec(ctx, create); createError != nil {
		if _, rollbackError := transaction.Exec(ctx, "ROLLBACK TO SAVEPOINT create_partition"); rollbackError != nil {
			return fmt.Errorf("roll back %s: %w", name, rollbackError)
		}
		var postgresError *pgconn.PgError
		if !errors.As(createError, &postgresError) || postgresError.Code != "23514" {
			return fmt.Errorf("create %s: %w", name, createError)
		}
	}
	if _, releaseError := transaction.Exec(ctx, "RELEASE SAVEPOINT create_partition"); releaseError != nil {
		return fmt.Errorf("release savepoint for %s: %w", name, releaseError)
	}
	return nil
}

// expiredPartitions lists the daily partitions of `table` whose whole day ended
// at or before `cutoff`.
func (store *PartitionStore) expiredPartitions(ctx context.Context, table string, cutoff time.Time) ([]string, error) {
	names, queryError := store.queries.ListChildPartitions(ctx, table)
	if queryError != nil {
		return nil, fmt.Errorf("list %s partitions: %w", table, queryError)
	}
	var expired []string
	for _, name := range names {
		if endOfDay, isDaily := partitionEnd(table, name); isDaily && !endOfDay.After(cutoff) {
			expired = append(expired, name)
		}
	}
	return expired, nil
}

// partitionEnd returns the exclusive end of a daily partition's range, or false
// if `name` is not a daily partition of `table`.
func partitionEnd(table, name string) (time.Time, bool) {
	match := dailyPartitionName.FindStringSubmatch(name)
	if match == nil || !strings.HasPrefix(name, table+"_p") {
		return time.Time{}, false
	}
	day, parseError := time.Parse(partitionNameDateLayout, match[1])
	if parseError != nil {
		return time.Time{}, false
	}
	return day.AddDate(0, 0, 1), true
}

// rowCounter counts the rows of a relation that is about to be dropped.
type rowCounter func(ctx context.Context, relation string) (int64, error)

// dropExpiredPartitions drops every expired daily partition of `tables`.
// It returns how many rows the first table held in the dropped partitions:
// counting costs one scan per partition, but keeps the retention "deleted"
// signal meaningful, and the drop itself is a metadata-only operation.
//
// The count is only a signal, so a failed count must never keep the partition:
// a full scan of a large day can outlast statement_timeout, and retrying that
// scan forever would stop the drop and fill the disk. Such a partition is
// dropped uncounted. A cancelled context still stops the run.
func (store *PartitionStore) dropExpiredPartitions(ctx context.Context, tables []string, cutoff time.Time, count rowCounter) (int64, error) {
	var rowsDropped int64
	for index, table := range tables {
		partitions, listError := store.expiredPartitions(ctx, table, cutoff)
		if listError != nil {
			return rowsDropped, listError
		}
		for _, partition := range partitions {
			if index == 0 {
				counted, countError := count(ctx, partition)
				if countError == nil {
					rowsDropped += counted
				} else if ctx.Err() != nil {
					return rowsDropped, ctx.Err()
				}
			}
			if _, dropError := store.pool.Exec(ctx, "DROP TABLE IF EXISTS "+identifier(partition)); dropError != nil {
				return rowsDropped, fmt.Errorf("drop %s: %w", partition, dropError)
			}
		}
	}
	return rowsDropped, nil
}

func (store *PartitionStore) countRows(ctx context.Context, relation string) (int64, error) {
	var count int64
	// A nested transaction is a savepoint: an optional count timing out must
	// not abort the enclosing retention transaction and prevent the drop.
	countError := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM "+identifier(relation)).Scan(&count)
	})
	if countError != nil {
		return 0, fmt.Errorf("count %s: %w", relation, countError)
	}
	return count, nil
}

// identifier quotes a table name for DDL, where it cannot be a parameter.
func identifier(name string) string {
	return pgx.Identifier{name}.Sanitize()
}
