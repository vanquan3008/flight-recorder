package postgres

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
// db/migrations/000001_init.up.sql; docs/ARCHITECTURE.md explains the design.
//
// Partitions are named <table>_pYYYYMMDD (UTC day). Rows whose day has no
// partition go to <table>_default, so a missing partition never rejects data.

// partitionedTables is every table that is partitioned by day.
var partitionedTables = []string{"events", "spans", "logs", "metric_data_points"}

// eventPartitionedTables expire together: a span or log is only useful next to
// its event, so all three are dropped on the same day boundary.
var eventPartitionedTables = []string{"events", "spans", "logs"}

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

func startOfUTCDay(moment time.Time) time.Time {
	year, month, day := moment.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// EnsurePartitions creates the daily partitions that cover `days` days starting
// at the UTC day of `from`, for every partitioned table. It is idempotent and
// safe to call from several replicas at once.
//
// A day that already has rows in a default partition cannot get its own
// partition (PostgreSQL would have to move those rows). That day keeps using
// the default partition and is skipped without an error.
func (database *Database) EnsurePartitions(ctx context.Context, from time.Time, days int) error {
	if days <= 0 {
		return fmt.Errorf("partition window must be positive, got %d days", days)
	}
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return fmt.Errorf("begin partition maintenance: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if _, lockError := transaction.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(partitionLockKey)); lockError != nil {
		return fmt.Errorf("lock partition maintenance: %w", lockError)
	}
	firstDay := startOfUTCDay(from)
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

// createDailyPartition creates one partition if it does not exist. A creation
// that PostgreSQL rejects (because the default partition already holds rows
// for that day) is rolled back to a savepoint so it cannot abort the other
// days in the same transaction.
func createDailyPartition(ctx context.Context, transaction pgx.Tx, table string, day time.Time) error {
	name := partitionName(table, day)
	// Table names and timestamps come from constants and time.Time, never from
	// request data, so formatting them into DDL is safe.
	create := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		name, table, day.Format(partitionBoundLayout), day.AddDate(0, 0, 1).Format(partitionBoundLayout))

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
func (database *Database) expiredPartitions(ctx context.Context, table string, cutoff time.Time) ([]string, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT child.relname
		FROM pg_inherits
		JOIN pg_class child ON child.oid = pg_inherits.inhrelid
		JOIN pg_class parent ON parent.oid = pg_inherits.inhparent
		WHERE parent.relname = $1 AND parent.relnamespace = current_schema()::regnamespace`, table)
	if queryError != nil {
		return nil, fmt.Errorf("list %s partitions: %w", table, queryError)
	}
	defer rows.Close()

	var expired []string
	for rows.Next() {
		var name string
		if scanError := rows.Scan(&name); scanError != nil {
			return nil, fmt.Errorf("scan %s partition: %w", table, scanError)
		}
		if endOfDay, isDaily := partitionEnd(table, name); isDaily && !endOfDay.After(cutoff) {
			expired = append(expired, name)
		}
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate %s partitions: %w", table, rowsError)
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

// dropExpiredPartitions drops every expired daily partition of `tables`.
// It returns how many rows the first table held in the dropped partitions:
// counting costs one scan per partition, but keeps the retention "deleted"
// signal meaningful, and the drop itself is a metadata-only operation.
func (database *Database) dropExpiredPartitions(ctx context.Context, tables []string, cutoff time.Time) (int64, error) {
	var rowsDropped int64
	for index, table := range tables {
		partitions, listError := database.expiredPartitions(ctx, table, cutoff)
		if listError != nil {
			return rowsDropped, listError
		}
		for _, partition := range partitions {
			if index == 0 {
				count, countError := database.countRows(ctx, partition)
				if countError != nil {
					return rowsDropped, countError
				}
				rowsDropped += count
			}
			if _, dropError := database.pool.Exec(ctx, "DROP TABLE IF EXISTS "+partition); dropError != nil {
				return rowsDropped, fmt.Errorf("drop %s: %w", partition, dropError)
			}
		}
	}
	return rowsDropped, nil
}

func (database *Database) countRows(ctx context.Context, relation string) (int64, error) {
	var count int64
	if countError := database.pool.QueryRow(ctx, "SELECT count(*) FROM "+relation).Scan(&count); countError != nil {
		return 0, fmt.Errorf("count %s: %w", relation, countError)
	}
	return count, nil
}
