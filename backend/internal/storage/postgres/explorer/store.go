// Package explorer reads traces, logs and request performance for the
// investigation screens.
package explorer

import (
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type QueryStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *QueryStore {
	return &QueryStore{pool: pool, queries: sqlcstore.New(pool)}
}
