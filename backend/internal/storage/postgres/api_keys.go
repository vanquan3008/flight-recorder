package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func (database *Database) CreateAPIKey(ctx context.Context, projectID, actorKeyID string, params auth.CreateAPIKeyParams) (auth.IssuedAPIKey, error) {
	now := time.Now().UTC()
	params, validationError := auth.NormalizeCreateAPIKeyParams(params, now)
	if validationError != nil || actorKeyID == "" {
		if validationError != nil {
			return auth.IssuedAPIKey{}, validationError
		}
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
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("begin API key create: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO api_keys (id, project_id, name, key_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		key.ID, projectID, key.Name, digest[:], key.Scopes, key.ExpiresAt, key.CreatedAt); insertError != nil {
		if isUniqueViolation(insertError) {
			return auth.IssuedAPIKey{}, auth.ErrAPIKeyNameExists
		}
		return auth.IssuedAPIKey{}, fmt.Errorf("create API key: %w", insertError)
	}
	if auditError := recordAPIKeyEvent(ctx, transaction, projectID, key.ID, actorKeyID, "CREATED", now); auditError != nil {
		return auth.IssuedAPIKey{}, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("commit API key create: %w", commitError)
	}
	return auth.IssuedAPIKey{APIKey: key, Key: rawKey}, nil
}

func (database *Database) ListAPIKeys(ctx context.Context, projectID string, filter auth.APIKeyFilter) ([]auth.APIKey, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, name, scopes, expires_at, created_at, revoked_at
		FROM api_keys
		WHERE project_id = $1
			AND ($2::timestamptz IS NULL OR created_at < $2 OR (created_at = $2 AND id < $3::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`, projectID, nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list API keys: %w", queryError)
	}
	defer rows.Close()
	keys := make([]auth.APIKey, 0, filter.Limit+1)
	for rows.Next() {
		var key auth.APIKey
		if scanError := rows.Scan(&key.ID, &key.Name, &key.Scopes, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt); scanError != nil {
			return nil, fmt.Errorf("scan API key: %w", scanError)
		}
		keys = append(keys, key)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate API keys: %w", rowsError)
	}
	return keys, nil
}

func (database *Database) ListAPIKeyEvents(ctx context.Context, projectID string, filter auth.APIKeyEventFilter) ([]auth.APIKeyEvent, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, api_key_id, actor_key_id, action, created_at
		FROM api_key_events
		WHERE project_id = $1
			AND ($2::timestamptz IS NULL OR created_at < $2 OR (created_at = $2 AND id < $3::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`, projectID, nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list API key events: %w", queryError)
	}
	defer rows.Close()
	events := make([]auth.APIKeyEvent, 0, filter.Limit+1)
	for rows.Next() {
		var event auth.APIKeyEvent
		if scanError := rows.Scan(&event.ID, &event.APIKeyID, &event.ActorKeyID, &event.Action, &event.CreatedAt); scanError != nil {
			return nil, fmt.Errorf("scan API key event: %w", scanError)
		}
		events = append(events, event)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate API key events: %w", rowsError)
	}
	return events, nil
}

func (database *Database) RotateAPIKey(ctx context.Context, projectID, keyID, actorKeyID string) (auth.IssuedAPIKey, error) {
	if keyID == "" || actorKeyID == "" {
		return auth.IssuedAPIKey{}, auth.ErrInvalidAPIKeyInput
	}
	rawKey, keyError := newRawAPIKey()
	if keyError != nil {
		return auth.IssuedAPIKey{}, keyError
	}
	digest := sha256.Sum256([]byte(rawKey))
	now := time.Now().UTC()
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("begin API key rotate: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var key auth.APIKey
	queryError := transaction.QueryRow(ctx, `
		SELECT id, name, scopes, expires_at, created_at, revoked_at
		FROM api_keys WHERE project_id = $1 AND id = $2 AND revoked_at IS NULL FOR UPDATE`, projectID, keyID).
		Scan(&key.ID, &key.Name, &key.Scopes, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.IssuedAPIKey{}, auth.ErrAPIKeyNotFound
	}
	if queryError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("load API key for rotate: %w", queryError)
	}
	if _, updateError := transaction.Exec(ctx, "UPDATE api_keys SET key_hash = $3 WHERE project_id = $1 AND id = $2", projectID, keyID, digest[:]); updateError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("rotate API key: %w", updateError)
	}
	if auditError := recordAPIKeyEvent(ctx, transaction, projectID, key.ID, actorKeyID, "ROTATED", now); auditError != nil {
		return auth.IssuedAPIKey{}, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return auth.IssuedAPIKey{}, fmt.Errorf("commit API key rotate: %w", commitError)
	}
	return auth.IssuedAPIKey{APIKey: key, Key: rawKey}, nil
}

func (database *Database) RevokeAPIKey(ctx context.Context, projectID, keyID, actorKeyID string) (bool, error) {
	if keyID == "" || actorKeyID == "" {
		return false, auth.ErrInvalidAPIKeyInput
	}
	now := time.Now().UTC()
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return false, fmt.Errorf("begin API key revoke: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	// Lock active admin keys in a stable order before checking the count. This
	// prevents two concurrent revocations from removing the last admin key.
	adminRows, adminQueryError := transaction.Query(ctx, `
		SELECT id FROM api_keys
		WHERE project_id = $1 AND revoked_at IS NULL AND 'admin' = ANY(scopes)
		ORDER BY id
		FOR UPDATE`, projectID)
	if adminQueryError != nil {
		return false, fmt.Errorf("lock admin API keys: %w", adminQueryError)
	}
	adminKeyIDs := make(map[string]struct{})
	for adminRows.Next() {
		var adminKeyID string
		if scanError := adminRows.Scan(&adminKeyID); scanError != nil {
			adminRows.Close()
			return false, fmt.Errorf("scan admin API key: %w", scanError)
		}
		adminKeyIDs[adminKeyID] = struct{}{}
	}
	if rowsError := adminRows.Err(); rowsError != nil {
		adminRows.Close()
		return false, fmt.Errorf("iterate admin API keys: %w", rowsError)
	}
	adminRows.Close()

	if _, isAdmin := adminKeyIDs[keyID]; isAdmin && len(adminKeyIDs) <= 1 {
		return false, auth.ErrLastAdminAPIKey
	}
	var revokedID string
	queryError := transaction.QueryRow(ctx, `
		UPDATE api_keys SET revoked_at = $3
		WHERE project_id = $1 AND id = $2 AND revoked_at IS NULL
		RETURNING id`, projectID, keyID, now).Scan(&revokedID)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return false, nil
	}
	if queryError != nil {
		return false, fmt.Errorf("revoke API key: %w", queryError)
	}
	if auditError := recordAPIKeyEvent(ctx, transaction, projectID, revokedID, actorKeyID, "REVOKED", now); auditError != nil {
		return false, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return false, fmt.Errorf("commit API key revoke: %w", commitError)
	}
	return true, nil
}

func newRawAPIKey() (string, error) {
	secret := make([]byte, 32)
	if _, readError := rand.Read(secret); readError != nil {
		return "", fmt.Errorf("generate API key: %w", readError)
	}
	return "tracepilot_" + base64.RawURLEncoding.EncodeToString(secret), nil
}

func recordAPIKeyEvent(ctx context.Context, transaction pgx.Tx, projectID, keyID, actorKeyID, action string, createdAt time.Time) error {
	eventID := telemetry.DeterministicID(createdAt, "api-key-event", projectID, keyID, actorKeyID, action)
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO api_key_events (id, project_id, api_key_id, actor_key_id, action, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, eventID, projectID, keyID, actorKeyID, action, createdAt); insertError != nil {
		return fmt.Errorf("record API key event: %w", insertError)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
