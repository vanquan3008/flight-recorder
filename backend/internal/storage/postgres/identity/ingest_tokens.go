package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/xslices"

	"github.com/jackc/pgx/v5"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func ingestTokenFrom(row sqlcstore.IngestToken) auth.IngestToken {
	return auth.IngestToken{ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt}
}

// CreateIngestToken records a telemetry sender's credential and has sign produce its JWT. The JWT is
// returned to the caller once and not stored: the row is what lets an owner list and revoke it.
func (store *IdentityStore) CreateIngestToken(ctx context.Context, projectID string, actor auth.Actor, params auth.CreateIngestTokenParams, sign func(tokenID string, expiresAt time.Time) (string, error)) (auth.IssuedIngestToken, error) {
	params, validationError := auth.NormalizeCreateIngestTokenParams(params, time.Now().UTC())
	if validationError != nil {
		return auth.IssuedIngestToken{}, validationError
	}
	var issued auth.IssuedIngestToken
	createError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		row, insertError := queries.InsertIngestToken(ctx, sqlcstore.InsertIngestTokenParams{
			ID: newID(), ProjectID: projectID, Name: params.Name, CreatedBy: pgvalue.OrNil(actor.UserID), ExpiresAt: params.ExpiresAt,
		})
		if pgvalue.IsUniqueViolation(insertError) {
			return auth.ErrIngestTokenExists
		}
		if insertError != nil {
			return insertError
		}
		issued.Token = ingestTokenFrom(row)
		jwt, signError := sign(row.ID, row.ExpiresAt)
		if signError != nil {
			return signError
		}
		issued.JWT = jwt
		return audit(ctx, queries, projectID, actor, "ingest_token.created", "ingest_token", row.ID, map[string]any{"name": row.Name, "expires_at": row.ExpiresAt})
	})
	if createError != nil {
		return auth.IssuedIngestToken{}, fmt.Errorf("create ingest token: %w", createError)
	}
	return issued, nil
}

func (store *IdentityStore) ListIngestTokens(ctx context.Context, projectID string, filter auth.IngestTokenFilter) ([]auth.IngestToken, error) {
	rows, queryError := store.queries.ListIngestTokens(ctx, sqlcstore.ListIngestTokensParams{
		ProjectID: projectID, Before: pgvalue.TimeOrNil(filter.Before), CursorID: pgvalue.OrNil(filter.CursorID), RowLimit: int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list ingest tokens: %w", queryError)
	}
	return xslices.Map(rows, ingestTokenFrom), nil
}

// GetIngestToken finds a token by ID; the receiver uses it to refuse revoked ones.
func (store *IdentityStore) GetIngestToken(ctx context.Context, tokenID string) (auth.IngestToken, error) {
	row, queryError := store.queries.GetIngestToken(ctx, tokenID)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.IngestToken{}, auth.ErrIngestTokenNotFound
	}
	if queryError != nil {
		return auth.IngestToken{}, fmt.Errorf("get ingest token: %w", queryError)
	}
	return ingestTokenFrom(row), nil
}

// RevokeIngestToken stops a token from working. It reports ErrIngestTokenNotFound when the token is not
// in the project or was already revoked.
func (store *IdentityStore) RevokeIngestToken(ctx context.Context, projectID, tokenID string, actor auth.Actor) error {
	revokeError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		if _, updateError := queries.RevokeIngestToken(ctx, sqlcstore.RevokeIngestTokenParams{ProjectID: projectID, ID: tokenID}); updateError != nil {
			if errors.Is(updateError, pgx.ErrNoRows) {
				return auth.ErrIngestTokenNotFound
			}
			return updateError
		}
		return audit(ctx, queries, projectID, actor, "ingest_token.revoked", "ingest_token", tokenID, nil)
	})
	if revokeError != nil {
		return fmt.Errorf("revoke ingest token: %w", revokeError)
	}
	return nil
}
