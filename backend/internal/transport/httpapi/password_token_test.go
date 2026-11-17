package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

type passwordBackend struct {
	*identityBackend
	changed bool
}

func (b *passwordBackend) GetCredentialsByID(context.Context, string) (auth.Credentials, error) {
	return b.credentials, nil
}

func (b *passwordBackend) SetPassword(context.Context, string, string, bool, string, auth.Actor) error {
	b.changed = true
	b.user.TokensValidAfter = time.Now().UTC().Truncate(time.Microsecond)
	return nil
}

func TestPasswordChangePreservesConfiguredTokenPolicy(t *testing.T) {
	hash, err := auth.HashPassword("old-audit-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	backend := &passwordBackend{identityBackend: newIdentityBackend(auth.RoleUser)}
	backend.credentials = auth.Credentials{User: backend.user, PasswordHash: hash}
	server := newTestServer(nil)
	server.backend, server.issuer, server.audience, server.accessTokenTTL = backend, "audit-issuer", "audit-console", 3*time.Minute
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(`{"current_password":"old-audit-passphrase","new_password":"new-audit-passphrase"}`))
	response := httptest.NewRecorder()
	server.authChangePassword(response, request, auth.Principal{UserID: backend.user.ID})
	if response.Code != http.StatusOK || !backend.changed {
		t.Fatalf("password change: %d %s", response.Code, response.Body)
	}
	var view sessionView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	claims, err := auth.VerifyJWTWithIssuerAudience(server.jwtSecret, view.AccessToken, auth.TokenAccess, server.issuer, server.audience, time.Now())
	if err != nil {
		t.Fatalf("new access token rejected: %v", err)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) != server.accessTokenTTL {
		t.Fatalf("wrong token lifetime: %v", claims.ExpiresAt.Sub(claims.IssuedAt))
	}
	if claims.TokenVersion != backend.user.TokenVersion() || claims.TokenVersion == 0 {
		t.Fatalf("password response kept an old token version: %d", claims.TokenVersion)
	}
}
