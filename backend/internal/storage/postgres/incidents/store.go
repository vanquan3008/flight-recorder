// Package incidents records incident occurrences inside the ingest
// transaction, changes incident status, and reads incidents with their history.
package incidents

import (
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type IncidentStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *IncidentStore {
	return &IncidentStore{pool: pool, queries: sqlcstore.New(pool)}
}
