// Package maintenance creates daily telemetry partitions and expires data
// past the retention cutoff.
package maintenance

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// Both a pool and a transaction can run the existing partition helpers.
type partitionConnection interface {
	sqlcstore.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

type PartitionStore struct {
	pool    partitionConnection
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *PartitionStore {
	return &PartitionStore{pool: pool, queries: sqlcstore.New(pool)}
}
