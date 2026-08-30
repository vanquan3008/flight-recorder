package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
)

var (
	ErrAPIKeyNotFound     = errors.New("API key not found")
	ErrAPIKeyNameExists   = errors.New("API key name already exists")
	ErrInvalidAPIKeyInput = errors.New("invalid API key input")
	ErrLastAdminAPIKey    = errors.New("cannot revoke the last admin API key")
)

var apiKeyNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

var allowedAPIKeyScopes = map[string]struct{}{
	"admin":  {},
	"ingest": {},
	"read":   {},
	"replay": {},
	"write":  {},
}

type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type IssuedAPIKey struct {
	APIKey APIKey
	Key    string
}

type CreateAPIKeyParams struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type APIKeyFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}

func NormalizeAPIKeyScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrInvalidAPIKeyInput)
	}
	normalized := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if _, allowed := allowedAPIKeyScopes[scope]; !allowed {
			return nil, fmt.Errorf("%w: scope %q is not allowed", ErrInvalidAPIKeyInput, scope)
		}
		if _, duplicate := seen[scope]; duplicate {
			return nil, fmt.Errorf("%w: duplicate scope %q", ErrInvalidAPIKeyInput, scope)
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeAPIKeyParams(params CreateAPIKeyParams, now time.Time) (CreateAPIKeyParams, error) {
	params.Name = strings.TrimSpace(params.Name)
	if !apiKeyNamePattern.MatchString(params.Name) {
		return CreateAPIKeyParams{}, fmt.Errorf("%w: name must be 1-64 letters, digits, spaces, dots, underscores, or hyphens", ErrInvalidAPIKeyInput)
	}
	scopes, scopeError := NormalizeAPIKeyScopes(params.Scopes)
	if scopeError != nil {
		return CreateAPIKeyParams{}, scopeError
	}
	if params.ExpiresAt != nil {
		expiresAt := params.ExpiresAt.UTC()
		if !expiresAt.After(now) {
			return CreateAPIKeyParams{}, fmt.Errorf("%w: expires_at must be in the future", ErrInvalidAPIKeyInput)
		}
		params.ExpiresAt = &expiresAt
	}
	params.Scopes = scopes
	return params, nil
}

func (database *Database) CreateAPIKey(ctx context.Context, projectID, actorKeyID string, params CreateAPIKeyParams) (IssuedAPIKey, error) {
	now := time.Now().UTC()
	params, validationError := normalizeAPIKeyParams(params, now)
	if validationError != nil || actorKeyID == "" {
		if validationError != nil {
			return IssuedAPIKey{}, validationError
		}
		return IssuedAPIKey{}, ErrInvalidAPIKeyInput
	}
	rawKey, keyError := newRawAPIKey()
	if keyError != nil {
		return IssuedAPIKey{}, keyError
	}
	digest := sha256.Sum256([]byte(rawKey))
	key := APIKey{
		ID: telemetry.DeterministicID(now, "api-key", projectID, params.Name, rawKey), Name: params.Name,
		Scopes: params.Scopes, ExpiresAt: params.ExpiresAt, CreatedAt: now,
	}
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return IssuedAPIKey{}, fmt.Errorf("begin API key create: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO api_keys (id, project_id, name, key_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		key.ID, projectID, key.Name, digest[:], key.Scopes, key.ExpiresAt, key.CreatedAt); insertError != nil {
		if isUniqueViolation(insertError) {
			return IssuedAPIKey{}, ErrAPIKeyNameExists
		}
		return IssuedAPIKey{}, fmt.Errorf("create API key: %w", insertError)
	}
	if auditError := recordAPIKeyEvent(ctx, transaction, projectID, key.ID, actorKeyID, "CREATED", now); auditError != nil {
		return IssuedAPIKey{}, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return IssuedAPIKey{}, fmt.Errorf("commit API key create: %w", commitError)
	}
	return IssuedAPIKey{APIKey: key, Key: rawKey}, nil
}

func (database *Database) ListAPIKeys(ctx context.Context, projectID string, filter APIKeyFilter) ([]APIKey, error) {
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
	keys := make([]APIKey, 0, filter.Limit+1)
	for rows.Next() {
		var key APIKey
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

func (database *Database) RotateAPIKey(ctx context.Context, projectID, keyID, actorKeyID string) (IssuedAPIKey, error) {
	if keyID == "" || actorKeyID == "" {
		return IssuedAPIKey{}, ErrInvalidAPIKeyInput
	}
	rawKey, keyError := newRawAPIKey()
	if keyError != nil {
		return IssuedAPIKey{}, keyError
	}
	digest := sha256.Sum256([]byte(rawKey))
	now := time.Now().UTC()
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return IssuedAPIKey{}, fmt.Errorf("begin API key rotate: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var key APIKey
	queryError := transaction.QueryRow(ctx, `
		SELECT id, name, scopes, expires_at, created_at, revoked_at
		FROM api_keys WHERE project_id = $1 AND id = $2 AND revoked_at IS NULL FOR UPDATE`, projectID, keyID).
		Scan(&key.ID, &key.Name, &key.Scopes, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return IssuedAPIKey{}, ErrAPIKeyNotFound
	}
	if queryError != nil {
		return IssuedAPIKey{}, fmt.Errorf("load API key for rotate: %w", queryError)
	}
	if _, updateError := transaction.Exec(ctx, "UPDATE api_keys SET key_hash = $3 WHERE project_id = $1 AND id = $2", projectID, keyID, digest[:]); updateError != nil {
		return IssuedAPIKey{}, fmt.Errorf("rotate API key: %w", updateError)
	}
	if auditError := recordAPIKeyEvent(ctx, transaction, projectID, key.ID, actorKeyID, "ROTATED", now); auditError != nil {
		return IssuedAPIKey{}, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return IssuedAPIKey{}, fmt.Errorf("commit API key rotate: %w", commitError)
	}
	return IssuedAPIKey{APIKey: key, Key: rawKey}, nil
}

func (database *Database) RevokeAPIKey(ctx context.Context, projectID, keyID, actorKeyID string) (bool, error) {
	if keyID == "" || actorKeyID == "" {
		return false, ErrInvalidAPIKeyInput
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
		return false, ErrLastAdminAPIKey
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
