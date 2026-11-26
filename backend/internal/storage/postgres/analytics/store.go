// Package analytics reads services, topology and metric series, detects
// deviations from baseline and assembles incident RCA input.
package analytics

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type AnalyticsStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *AnalyticsStore {
	return &AnalyticsStore{pool: pool, queries: sqlcstore.New(pool)}
}
