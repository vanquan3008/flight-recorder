package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *KeyStore) CreateAPIKey(ctx context.Context, projectID, actorKeyID string, params auth.CreateAPIKeyParams) (auth.IssuedAPIKey, error) {
	now := time.Now().UTC()
	params, validationError := auth.NormalizeCreateAPIKeyParams(params, now)
	if validationError != nil {
		return auth.IssuedAPIKey{}, validationError
	}
	if actorKeyID == "" {
		return auth.IssuedAPIKey{}, auth.ErrInvalidAPIKeyInput
	}
	rawKey, keyError := newRawAPIKey()
	if keyError != nil {
		return auth.IssuedAPIKey{}, keyError
	}
	digest := sha256.Sum256([]byte(rawKey))
	key := auth.APIKey{
		ID: telemetry.DeterministicID(now, "api-key", projectID, params.Name, rawKey), Name: params.Name,
		Scopes: params.Scopes, ExpiresAt: params.ExpiresAt, CreatedAt: now,
	}
	createError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		if insertError := queries.InsertAPIKey(ctx, sqlcstore.InsertAPIKeyParams{
			ID: key.ID, ProjectID: projectID, Name: key.Name, KeyHash: digest[:],
			Scopes: key.Scopes, ExpiresAt: key.ExpiresAt, CreatedAt: key.CreatedAt,
		}); insertError != nil {
			if pgvalue.IsUniqueViolation(insertError) {
				return auth.ErrAPIKeyNameExists
			}
			return fmt.Errorf("insert API key: %w", insertError)
		}
		return recordAPIKeyEvent(ctx, queries, projectID, key.ID, actorKeyID, "CREATED", now)
	})
	if createError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("create API key: %w", createError)
	}
	return auth.IssuedAPIKey{APIKey: key, Key: rawKey}, nil
}

func (store *KeyStore) ListAPIKeys(ctx context.Context, projectID string, filter auth.APIKeyFilter) ([]auth.APIKey, error) {
	rows, queryError := store.queries.ListAPIKeys(ctx, sqlcstore.ListAPIKeysParams{
		ProjectID: projectID,
		Before:    pgvalue.TimeOrNil(filter.Before),
		CursorID:  pgvalue.OrNil(filter.CursorID),
		RowLimit:  int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list API keys: %w", queryError)
	}
	keys := make([]auth.APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, auth.APIKey{
			ID: row.ID, Name: row.Name, Scopes: row.Scopes,
			ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, RevokedAt: row.RevokedAt,
		})
	}
	return keys, nil
}

func (store *KeyStore) ListAPIKeyEvents(ctx context.Context, projectID string, filter auth.APIKeyEventFilter) ([]auth.APIKeyEvent, error) {
	rows, queryError := store.queries.ListAPIKeyEvents(ctx, sqlcstore.ListAPIKeyEventsParams{
		ProjectID: projectID,
		Before:    pgvalue.TimeOrNil(filter.Before),
		CursorID:  pgvalue.OrNil(filter.CursorID),
		RowLimit:  int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list API key events: %w", queryError)
	}
	events := make([]auth.APIKeyEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, auth.APIKeyEvent{
			ID: row.ID, APIKeyID: row.APIKeyID, ActorKeyID: row.ActorKeyID,
			Action: row.Action, CreatedAt: row.CreatedAt,
		})
	}
	return events, nil
}

func (store *KeyStore) RotateAPIKey(ctx context.Context, projectID, keyID, actorKeyID string) (auth.IssuedAPIKey, error) {
	if keyID == "" || actorKeyID == "" {
		return auth.IssuedAPIKey{}, auth.ErrInvalidAPIKeyInput
	}
	rawKey, keyError := newRawAPIKey()
	if keyError != nil {
		return auth.IssuedAPIKey{}, keyError
	}
	digest := sha256.Sum256([]byte(rawKey))
	now := time.Now().UTC()
	var key auth.APIKey
	rotateError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		row, lockError := queries.LockActiveAPIKey(ctx, sqlcstore.LockActiveAPIKeyParams{ProjectID: projectID, ID: keyID})
		if errors.Is(lockError, pgx.ErrNoRows) {
			return auth.ErrAPIKeyNotFound
		}
		if lockError != nil {
			return fmt.Errorf("load API key: %w", lockError)
		}
		key = auth.APIKey{
			ID: row.ID, Name: row.Name, Scopes: row.Scopes,
			ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, RevokedAt: row.RevokedAt,
		}
		if updateError := queries.UpdateAPIKeyHash(ctx, sqlcstore.UpdateAPIKeyHashParams{ProjectID: projectID, ID: keyID, KeyHash: digest[:]}); updateError != nil {
			return fmt.Errorf("update API key hash: %w", updateError)
		}
		return recordAPIKeyEvent(ctx, queries, projectID, key.ID, actorKeyID, "ROTATED", now)
	})
	if rotateError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("rotate API key: %w", rotateError)
	}
	return auth.IssuedAPIKey{APIKey: key, Key: rawKey}, nil
}

// RevokeAPIKey reports false when the key does not exist or is already revoked.
func (store *KeyStore) RevokeAPIKey(ctx context.Context, projectID, keyID, actorKeyID string) (bool, error) {
	if keyID == "" || actorKeyID == "" {
		return false, auth.ErrInvalidAPIKeyInput
	}
	now := time.Now().UTC()
	revoked := false
	revokeError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		adminKeyIDs, lockError := queries.LockActiveAdminAPIKeyIDs(ctx, projectID)
		if lockError != nil {
			return fmt.Errorf("lock admin API keys: %w", lockError)
		}
		if slices.Contains(adminKeyIDs, keyID) && len(adminKeyIDs) <= 1 {
			return auth.ErrLastAdminAPIKey
		}
		revokedID, updateError := queries.RevokeAPIKey(ctx, sqlcstore.RevokeAPIKeyParams{ProjectID: projectID, ID: keyID, RevokedAt: now})
		if errors.Is(updateError, pgx.ErrNoRows) {
			return nil
		}
		if updateError != nil {
			return fmt.Errorf("update API key: %w", updateError)
		}
		revoked = true
		return recordAPIKeyEvent(ctx, queries, projectID, revokedID, actorKeyID, "REVOKED", now)
	})
	if revokeError != nil {
		return false, fmt.Errorf("revoke API key: %w", revokeError)
	}
	return revoked, nil
}

func newRawAPIKey() (string, error) {
	secret := make([]byte, 32)
	if _, readError := rand.Read(secret); readError != nil {
		return "", fmt.Errorf("generate API key: %w", readError)
	}
	return "tracepilot_" + base64.RawURLEncoding.EncodeToString(secret), nil
}

func recordAPIKeyEvent(ctx context.Context, queries *sqlcstore.Queries, projectID, keyID, actorKeyID, action string, createdAt time.Time) error {
	if insertError := queries.InsertAPIKeyEvent(ctx, sqlcstore.InsertAPIKeyEventParams{
		ID:        telemetry.DeterministicID(createdAt, "api-key-event", projectID, keyID, actorKeyID, action),
		ProjectID: projectID, APIKeyID: keyID, ActorKeyID: actorKeyID, Action: action, CreatedAt: createdAt,
	}); insertError != nil {
		return fmt.Errorf("record API key event: %w", insertError)
	}
	return nil
}
