// Package signals records the envelopes of failing spans and logs and the
// incident occurrences they open. Telemetry itself is stored in ClickHouse.
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
