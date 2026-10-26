// Package maintenance creates daily telemetry partitions and expires data
// past the retention cutoff.
package maintenance

import (
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type PartitionStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *PartitionStore {
	return &PartitionStore{pool: pool, queries: sqlcstore.New(pool)}
}
