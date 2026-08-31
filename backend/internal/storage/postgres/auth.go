package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

var ErrUnauthorized = errors.New("unauthorized")

type Principal struct {
	KeyID     string
	ProjectID string
	Scopes    []string
}

func (principal Principal) HasScope(wanted string) bool {
	for _, scope := range principal.Scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func (database *Database) EnsureBootstrapAPIKey(ctx context.Context, projectID, rawKey string) error {
	if rawKey == "" {
		return nil
	}
	digest := sha256.Sum256([]byte(rawKey))
	keyID := telemetry.DeterministicID(time.Unix(0, 0), projectID, string(digest[:]))
	if _, executeError := database.pool.Exec(ctx, `
		INSERT INTO api_keys (id, project_id, name, key_hash, scopes)
		VALUES ($1, $2, 'bootstrap', $3, ARRAY['read', 'write', 'ingest', 'replay', 'admin'])
		ON CONFLICT (project_id, name) DO UPDATE
		SET key_hash = EXCLUDED.key_hash, scopes = EXCLUDED.scopes, revoked_at = NULL`, keyID, projectID, digest[:]); executeError != nil {
		return fmt.Errorf("ensure bootstrap API key: %w", executeError)
	}
	return nil
}

func (database *Database) AuthenticateAPIKey(ctx context.Context, rawKey string) (Principal, error) {
	if rawKey == "" {
		return Principal{}, ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(rawKey))
	var principal Principal
	queryError := database.pool.QueryRow(ctx, `
		SELECT id, project_id, scopes
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`, digest[:]).Scan(&principal.KeyID, &principal.ProjectID, &principal.Scopes)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthorized
	}
	if queryError != nil {
		return Principal{}, fmt.Errorf("authenticate API key: %w", queryError)
	}
	return principal, nil
}
