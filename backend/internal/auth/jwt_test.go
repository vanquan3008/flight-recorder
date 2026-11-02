package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestJWTRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	token, expires, err := IssueAccessToken(testSecret, "user-1", now)
	if err != nil || !expires.Equal(now.Add(AccessTokenTTL)) {
		t.Fatalf("token error = %v, expires = %v", err, expires)
	}
	claims, err := VerifyJWT(testSecret, token, TokenAccess, now.Add(time.Minute))
	if err != nil || claims.Subject != "user-1" || claims.Kind != TokenAccess || !claims.IssuedAt.Equal(now) {
		t.Fatalf("claims = %+v, error = %v", claims, err)
	}
}

func TestJWTRefusesWhatIsNotExactlyOurs(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	token, _, _ := IssueAccessToken(testSecret, "user-1", now)
	parts := strings.Split(token, ".")
	b64 := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
	other, _, _ := IssueAccessToken([]byte("another-secret-another-secret-0000"), "user-1", now)
	cases := map[string]string{
		"wrong secret":      other,
		"tampered payload":  parts[0] + "." + b64(`{"iss":"tracepilot","aud":"tracepilot","sub":"admin","tp":"access","iat":1,"exp":99999999999}`) + "." + parts[2],
		"alg none":          b64(`{"alg":"none","typ":"JWT"}`) + "." + parts[1] + ".",
		"another algorithm": b64(`{"alg":"HS512","typ":"JWT"}`) + "." + parts[1] + "." + parts[2],
		"missing signature": parts[0] + "." + parts[1],
		"garbage":           "not-a-token",
		"empty":             "",
		"extra segment":     token + ".x",
	}
	for name, candidate := range cases {
		if _, err := VerifyJWT(testSecret, candidate, TokenAccess, now); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: error = %v, want ErrInvalidToken", name, err)
		}
	}
}

func TestJWTExpiresAndKindsDoNotMix(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	access, _, _ := IssueAccessToken(testSecret, "user-1", now)
	if _, err := VerifyJWT(testSecret, access, TokenAccess, now.Add(AccessTokenTTL+time.Minute)); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("after expiry: error = %v, want ErrExpiredToken", err)
	}
	if _, err := VerifyJWT(testSecret, access, TokenAccess, now.Add(-time.Hour)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("issued in the future: error = %v, want ErrInvalidToken", err)
	}
	ingest, err := IssueIngestToken(testSecret, "project-1", "token-1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyJWT(testSecret, ingest, TokenAccess, now); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("an ingest token must not open the console: %v", err)
	}
	if _, err := VerifyJWT(testSecret, access, TokenIngest, now); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("an access token must not send telemetry: %v", err)
	}
	if _, err := SignJWT([]byte("short"), Claims{Kind: TokenAccess, Subject: "u", IssuedAt: now, ExpiresAt: now.Add(time.Hour)}); err == nil {
		t.Fatal("a short secret must be refused")
	}
}

type tokenRows map[string]IngestToken

func (rows tokenRows) GetIngestToken(_ context.Context, id string) (IngestToken, error) {
	if token, ok := rows[id]; ok {
		return token, nil
	}
	return IngestToken{}, ErrIngestTokenNotFound
}

func TestIngestTokenAuthentication(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	rows := tokenRows{"t1": {ID: "t1", ProjectID: "p1", ExpiresAt: expires}}
	good, _ := IssueIngestToken(testSecret, "p1", "t1", now, expires)
	principal, err := AuthenticateIngestToken(context.Background(), testSecret, rows, good, now)
	if err != nil || principal.ProjectID != "p1" || !principal.HasScope(ScopeIngest) || principal.HasScope(ScopeRead) || principal.UserID != "" {
		t.Fatalf("principal = %+v, error = %v; a sender may only ingest into its own project", principal, err)
	}

	revokedAt := now
	rows["t2"] = IngestToken{ID: "t2", ProjectID: "p1", ExpiresAt: expires, RevokedAt: &revokedAt}
	revoked, _ := IssueIngestToken(testSecret, "p1", "t2", now, expires)
	otherProject, _ := IssueIngestToken(testSecret, "p2", "t1", now, expires) // names a project the row does not belong to
	unknown, _ := IssueIngestToken(testSecret, "p1", "t9", now, expires)
	access, _, _ := IssueAccessToken(testSecret, "user-1", now)
	for name, token := range map[string]string{"revoked": revoked, "wrong project": otherProject, "unknown row": unknown, "an access token": access, "garbage": "x.y.z"} {
		if _, err := AuthenticateIngestToken(context.Background(), testSecret, rows, token, now); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("%s: error = %v, want ErrUnauthorized", name, err)
		}
	}
	if _, err := AuthenticateIngestToken(context.Background(), testSecret, rows, good, expires.Add(time.Minute)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("an expired token: error = %v, want ErrUnauthorized", err)
	}
}
