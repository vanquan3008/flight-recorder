// Package pgbatch lets sqlc-generated :exec queries queue into one pgx.Batch,
// so a write path keeps a single round trip while its SQL stays in .sql files.
package pgbatch

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var errReadInBatch = errors.New("pgbatch: queries that read rows cannot be queued")

// Queue implements the sqlc DBTX interface. Exec records the statement and
// always succeeds; execution errors are returned by Send.
type Queue struct {
	batch pgx.Batch
}

func (queue *Queue) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	queue.batch.Queue(sql, arguments...)
	return pgconn.CommandTag{}, nil
}

func (queue *Queue) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errReadInBatch
}

func (queue *Queue) QueryRow(context.Context, string, ...any) pgx.Row {
	return errorRow{}
}

func (queue *Queue) Len() int {
	return queue.batch.Len()
}

// Sender is satisfied by pgx.Tx and *pgxpool.Pool. A pool runs the batch in
// an implicit transaction.
type Sender interface {
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
}

// Send runs the queued statements in order and returns the first error.
func (queue *Queue) Send(ctx context.Context, sender Sender) error {
	if queue.batch.Len() == 0 {
		return nil
	}
	return sender.SendBatch(ctx, &queue.batch).Close()
}

type errorRow struct{}

func (errorRow) Scan(...any) error { return errReadInBatch }
