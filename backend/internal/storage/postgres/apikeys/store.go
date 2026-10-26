// Package apikeys issues, rotates and revokes API keys and authenticates
// requests by key hash.
package apikeys

import (
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type KeyStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *KeyStore {
	return &KeyStore{pool: pool, queries: sqlcstore.New(pool)}
}
