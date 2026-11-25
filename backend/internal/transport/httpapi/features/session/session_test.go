package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
)

type backend struct {
	Backend
	user        auth.User
	credentials auth.Credentials
	changed     bool
}

func (b *backend) GetUser(context.Context, string) (auth.User, error) { return b.user, nil }

func (b *backend) GetCredentialsByID(context.Context, string) (auth.Credentials, error) {
	return b.credentials, nil
}

func (b *backend) SetPassword(context.Context, string, string, bool, string, auth.Actor) error {
	b.changed = true
	b.user.TokensValidAfter = time.Now().UTC().Truncate(time.Microsecond)
	return nil
}

func (b *backend) GetCredentials(context.Context, string) (auth.Credentials, error) {
	return auth.Credentials{}, auth.ErrUserNotFound
}

func (b *backend) ListMemberships(context.Context, string) ([]auth.Membership, error) {
	return nil, nil
}

func TestPasswordChangePreservesConfiguredTokenPolicy(t *testing.T) {
	hash, err := auth.HashPassword("old-audit-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	user := auth.User{ID: "user", Email: "alice@example.com"}
	store := &backend{user: user, credentials: auth.Credentials{User: user, PasswordHash: hash}}
	configuration := Config{TokenSecret: []byte("0123456789abcdef0123456789abcdef"), TokenIssuer: "audit-issuer", TokenAudience: "audit-console", AccessTokenTTL: 3 * time.Minute}
	handler := New(configuration, store, middleware.NewRequestRateLimiter(time.Hour), nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(`{"current_password":"old-audit-passphrase","new_password":"new-audit-passphrase"}`))
	response := httptest.NewRecorder()
	handler.changePassword(response, request, auth.Principal{UserID: user.ID})
	if response.Code != http.StatusOK || !store.changed {
		t.Fatalf("password change: %d %s", response.Code, response.Body)
	}
	var view sessionView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	claims, err := auth.VerifyJWTWithIssuerAudience(configuration.TokenSecret, view.AccessToken, auth.TokenAccess, configuration.TokenIssuer, configuration.TokenAudience, time.Now())
	if err != nil {
		t.Fatalf("new access token rejected: %v", err)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) != configuration.AccessTokenTTL {
		t.Fatalf("wrong token lifetime: %v", claims.ExpiresAt.Sub(claims.IssuedAt))
	}
	if claims.TokenVersion != store.user.TokenVersion() || claims.TokenVersion == 0 {
		t.Fatalf("password response kept an old token version: %d", claims.TokenVersion)
	}
}

func TestRefreshCookieIsSecureWhereTheConnectionIs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config Config
		setup  func(*http.Request)
		secure bool
	}{
		{"plain development", Config{}, func(*http.Request) {}, false},
		{"production", Config{Production: true}, func(*http.Request) {}, true},
		{"TLS listener", Config{TLS: true}, func(*http.Request) {}, true},
		{"SameSite=None", Config{CookieSameSite: http.SameSiteNoneMode}, func(*http.Request) {}, true},
		{"untrusted forwarded HTTPS", Config{}, func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, false},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		tc.setup(request)
		cookie := New(tc.config, nil, nil, nil).refreshCookie(request, "token", 60)
		if cookie.Secure != tc.secure || !cookie.HttpOnly || cookie.Path != refreshPath {
			t.Fatalf("%s: cookie = %+v", tc.name, cookie)
		}
	}
}

func TestSignInDoesNotKeepAnOversizedEmailAsARateLimitKey(t *testing.T) {
	limiter := middleware.NewRequestRateLimiter(time.Hour)
	handler := New(Config{}, &backend{}, limiter, nil)
	body := `{"email":"` + strings.Repeat("a", 100_000) + `@example.com","password":"a-long-enough-passphrase"}`
	recorder := httptest.NewRecorder()
	handler.login(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)), auth.Principal{})
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatalf("status %d body %.200s, want 401 INVALID_CREDENTIALS", recorder.Code, recorder.Body)
	}
	clientKeys := limiter.ClientKeys()
	if len(clientKeys) == 0 {
		t.Fatal("the attempt was not counted")
	}
	for _, key := range clientKeys {
		if len(key) > len("login:")+maxEmailLength {
			t.Fatalf("a rate-limit key of %d bytes was stored", len(key))
		}
	}
}
