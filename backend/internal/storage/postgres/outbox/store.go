// Package outbox queues webhook deliveries inside the transaction that
// produced the domain event, and lets the notification worker claim them.
package outbox

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type OutboxStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
	// encryptionKey encrypts webhook secrets at rest; nil disables them.
	encryptionKey []byte
}

func New(pool *pgxpool.Pool, encryptionKey []byte) *OutboxStore {
	return &OutboxStore{pool: pool, queries: sqlcstore.New(pool), encryptionKey: encryptionKey}
}
