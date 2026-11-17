package auth

import (
	"context"
	"errors"
	"time"
)

// IssueAccessToken signs the short-lived token a signed-in console sends with each request.
func IssueAccessToken(secret []byte, userID string, now time.Time) (string, time.Time, error) {
	return IssueAccessTokenWithOptions(secret, userID, now, AccessTokenTTL, "", "", 0)
}

// IssueAccessTokenWithOptions signs the console token using custom TTL, issuer, and audience.
func IssueAccessTokenWithOptions(secret []byte, userID string, now time.Time, ttl time.Duration, issuer, audience string, tokenVersion int64) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = AccessTokenTTL
	}
	expires := now.Add(ttl)
	token, signError := SignJWT(secret, Claims{
		Kind:         TokenAccess,
		Subject:      userID,
		IssuedAt:     now,
		ExpiresAt:    expires,
		Issuer:       issuer,
		Audience:     audience,
		TokenVersion: tokenVersion,
	})
	return token, expires, signError
}

// IssueIngestToken signs a telemetry sender's credential for one project.
func IssueIngestToken(secret []byte, projectID, tokenID string, now, expires time.Time) (string, error) {
	return SignJWT(secret, Claims{Kind: TokenIngest, Subject: projectID, ID: tokenID, IssuedAt: now, ExpiresAt: expires})
}

// IngestTokenSource looks up the row behind an ingest token.
type IngestTokenSource interface {
	GetIngestToken(ctx context.Context, tokenID string) (IngestToken, error)
}

// AuthenticateIngestToken accepts a telemetry sender's JWT: the signature and expiry must hold and the
// token must not have been revoked. The project it may write to is the one it was issued for.
func AuthenticateIngestToken(ctx context.Context, secret []byte, source IngestTokenSource, raw string, now time.Time) (Principal, error) {
	claims, verifyError := VerifyJWT(secret, raw, TokenIngest, now)
	if verifyError != nil {
		return Principal{}, ErrUnauthorized
	}
	token, lookupError := source.GetIngestToken(ctx, claims.ID)
	if errors.Is(lookupError, ErrIngestTokenNotFound) {
		return Principal{}, ErrUnauthorized
	}
	if lookupError != nil {
		return Principal{}, lookupError
	}
	if !token.Active(now) || token.ProjectID != claims.Subject {
		return Principal{}, ErrUnauthorized
	}
	return Principal{ProjectID: token.ProjectID, Scopes: []string{ScopeIngest}, TokenID: token.ID}, nil
}
