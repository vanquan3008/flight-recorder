// Package signals writes ingested telemetry: events with their span or log
// record (and the incidents they open) in one transaction, and metric points.
package signals

import (
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type SignalStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *SignalStore {
	return &SignalStore{pool: pool, queries: sqlcstore.New(pool)}
}
