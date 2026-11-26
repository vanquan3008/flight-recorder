// Package webhooks manages webhook subscriptions, their encrypted secrets
// and delivery history.
package webhooks

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

type WebhookStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
	// encryptionKey encrypts webhook secrets at rest; nil disables them.
	encryptionKey []byte
}

func New(pool *pgxpool.Pool, encryptionKey []byte) *WebhookStore {
	return &WebhookStore{pool: pool, queries: sqlcstore.New(pool), encryptionKey: encryptionKey}
}
