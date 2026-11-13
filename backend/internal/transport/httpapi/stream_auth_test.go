package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestStreamRejectsAnExpiredTokenInsideClockSkew(t *testing.T) {
	server := newTestServer(nil)
	server.backend = newIdentityBackend(auth.RoleUser)
	now := time.Now()
	token, err := auth.SignJWT(server.jwtSecret, auth.Claims{Kind: auth.TokenAccess, Subject: "owner", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/live/stream", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(projectHeader, testProjectIDForRoutes)
	response := httptest.NewRecorder()
	server.requireScopeStreaming(auth.ScopeRead, func(http.ResponseWriter, *http.Request, auth.Principal) { t.Fatal("expired stream was admitted") })(response, request)
	if response.Code != http.StatusUnauthorized || errorCode(t, response) != "TOKEN_EXPIRED" {
		t.Fatalf("expired stream: %d %s", response.Code, response.Body)
	}
}

func TestStreamAccessRechecksAccountAndMembership(t *testing.T) {
	now := time.Now()
	claims := auth.Claims{Subject: "owner", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	principal := auth.Principal{UserID: "owner", ProjectID: "project"}
	cases := []struct {
		name    string
		change  func(*identityBackend, *auth.Claims)
		allowed bool
	}{
		{"member", func(*identityBackend, *auth.Claims) {}, true},
		{"removed member", func(b *identityBackend, _ *auth.Claims) { b.role = "" }, false},
		{"disabled account", func(b *identityBackend, _ *auth.Claims) { b.user.DisabledAt = &now }, false},
		{"changed password", func(b *identityBackend, _ *auth.Claims) { b.user.TokensValidAfter = now }, false},
		{"temporary password", func(b *identityBackend, _ *auth.Claims) { b.user.MustChangePassword = true }, false},
		{"expired token", func(_ *identityBackend, c *auth.Claims) { c.ExpiresAt = now.Add(-time.Second) }, false},
		{"administrator", func(b *identityBackend, _ *auth.Claims) { b.role = ""; b.user.PlatformAdmin = true }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := newIdentityBackend(auth.RoleUser)
			copyClaims := claims
			tc.change(backend, &copyClaims)
			server := &Server{backend: backend}
			if got := server.streamAccessAllowed(context.Background(), principal, copyClaims); got != tc.allowed {
				t.Fatalf("stream access = %v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestStreamContextEndsWithTokenAndRequest(t *testing.T) {
	server := &Server{}
	for _, expired := range []bool{false, true} {
		ctx, stop := context.WithCancel(context.Background())
		deadline := time.Now().Add(time.Hour)
		if expired {
			deadline = time.Now().Add(-time.Second)
		}
		streamCtx, cancel := server.watchStreamAccess(ctx, auth.Principal{}, auth.Claims{ExpiresAt: deadline})
		if !expired {
			stop()
		}
		select {
		case <-streamCtx.Done():
		case <-time.After(time.Second):
			t.Fatal("stream outlived its token or request")
		}
		cancel()
		stop()
	}
}
