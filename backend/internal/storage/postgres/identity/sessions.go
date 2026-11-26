package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vanquan3008/tracepilot/internal/auth"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// CreateSession starts a refresh session for a user and returns its opaque token, which the caller
// sets as an HttpOnly cookie. Only the token's digest is stored.
func (store *IdentityStore) CreateSession(ctx context.Context, userID, userAgent string) (string, auth.Session, error) {
	token, tokenError := newToken()
	if tokenError != nil {
		return "", auth.Session{}, tokenError
	}
	if len(userAgent) > 256 {
		userAgent = userAgent[:256]
	}
	now := time.Now().UTC()
	session := auth.Session{UserID: userID, ExpiresAt: now.Add(sessionIdleTTL)}
	if insertError := store.queries.InsertSession(ctx, sqlcstore.InsertSessionParams{
		TokenHash: hashToken(token), UserID: userID, CreatedAt: now,
		ExpiresAt: session.ExpiresAt, MaxExpiresAt: now.Add(sessionMaxTTL), UserAgent: userAgent,
	}); insertError != nil {
		return "", auth.Session{}, fmt.Errorf("create session: %w", insertError)
	}
	// Expired sessions are only dead weight; clear them as new ones arrive.
	_, _ = store.queries.DeleteExpiredSessions(ctx)
	return token, session, nil
}

// RefreshSession trades a refresh token for a new one and returns the user it belongs to. The old
// token stops working, so a stolen copy is good for one use at most, and one that is replayed after
// the owner refreshed is simply refused. A disabled account's session, or one past its idle or
// absolute lifetime, is not found.
func (store *IdentityStore) RefreshSession(ctx context.Context, token string) (string, auth.User, error) {
	if token == "" {
		return "", auth.User{}, auth.ErrSessionNotFound
	}
	digest := hashToken(token)
	row, queryError := store.queries.GetSessionUser(ctx, digest)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return "", auth.User{}, auth.ErrSessionNotFound
	}
	if queryError != nil {
		return "", auth.User{}, fmt.Errorf("find session: %w", queryError)
	}
	next, tokenError := newToken()
	if tokenError != nil {
		return "", auth.User{}, tokenError
	}
	now := time.Now().UTC()
	expires := now.Add(sessionIdleTTL)
	if expires.After(row.MaxExpiresAt) {
		expires = row.MaxExpiresAt
	}
	rotated, rotateError := store.queries.RotateSession(ctx, sqlcstore.RotateSessionParams{
		NewTokenHash: hashToken(next), LastSeenAt: now, ExpiresAt: expires, OldTokenHash: digest,
	})
	if rotateError != nil {
		return "", auth.User{}, fmt.Errorf("rotate session: %w", rotateError)
	}
	if rotated == 0 {
		// Another request refreshed the same token first.
		return "", auth.User{}, auth.ErrSessionNotFound
	}
	return next, userFrom(row.User), nil
}

// RevokeSession ends one session; an unknown token is not an error.
func (store *IdentityStore) RevokeSession(ctx context.Context, token string) error {
	if deleteError := store.queries.DeleteSession(ctx, hashToken(token)); deleteError != nil {
		return fmt.Errorf("revoke session: %w", deleteError)
	}
	return nil
}
