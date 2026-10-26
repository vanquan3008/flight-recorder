package apikeys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// bootstrapKeyLockKey serializes EnsureBootstrapAPIKey across replicas.
const bootstrapKeyLockKey = 7_203_411_003

// EnsureBootstrapAPIKey provisions the configured bootstrap key. A restart with
// the same key leaves the stored row untouched, so a revoked or narrowed
// bootstrap key stays that way. Only a different configured key rotates the
// stored hash and reactivates the row, which keeps lost-admin recovery possible.
func (store *KeyStore) EnsureBootstrapAPIKey(ctx context.Context, projectID, rawKey string) error {
	if rawKey == "" {
		return nil
	}
	digest := sha256.Sum256([]byte(rawKey))
	// Replicas started together upsert the same key; see EnsureIngestionScope.
	if executeError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		if lockError := queries.AcquireTransactionLock(ctx, bootstrapKeyLockKey); lockError != nil {
			return lockError
		}
		return queries.EnsureBootstrapAPIKey(ctx, sqlcstore.EnsureBootstrapAPIKeyParams{
			ID:        telemetry.DeterministicID(time.Unix(0, 0), projectID, string(digest[:])),
			ProjectID: projectID,
			KeyHash:   digest[:],
		})
	}); executeError != nil {
		return fmt.Errorf("ensure bootstrap API key: %w", executeError)
	}
	return nil
}

func (store *KeyStore) AuthenticateAPIKey(ctx context.Context, rawKey string) (auth.Principal, error) {
	if rawKey == "" {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(rawKey))
	row, queryError := store.queries.AuthenticateAPIKey(ctx, digest[:])
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	if queryError != nil {
		return auth.Principal{}, fmt.Errorf("authenticate API key: %w", queryError)
	}
	return auth.Principal{KeyID: row.ID, ProjectID: row.ProjectID, Scopes: row.Scopes}, nil
}
