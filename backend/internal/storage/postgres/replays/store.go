// Package replays stores request replay executions and their results.
package replays

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type ReplayStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *ReplayStore {
	return &ReplayStore{pool: pool, queries: sqlcstore.New(pool)}
}
