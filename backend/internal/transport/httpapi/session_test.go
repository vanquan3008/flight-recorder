package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func post(handler http.Handler, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func cookieNamed(recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestRegistrationOpensTheFirstAccountAsSuperAdministrator(t *testing.T) {
	backend := newIdentityBackend("")
	handler := identityServer(backend)
	body := `{"email":"first@example.com","display_name":"First","password":"a-long-enough-passphrase"}`
	response := post(handler, "/api/v1/auth/register", body, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", response.Code, response.Body)
	}
	var view struct {
		AccessToken string `json:"access_token"`
		User        struct {
			PlatformAdmin bool `json:"platform_admin"`
		} `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || view.AccessToken == "" || !view.User.PlatformAdmin {
		t.Fatalf("view = %+v (%v); the first account signs in as super administrator", view, err)
	}
	refresh := cookieNamed(response, refreshCookieName)
	if refresh == nil || !refresh.HttpOnly || refresh.SameSite != http.SameSiteStrictMode || refresh.Path != refreshPath || refresh.Value != "refresh-1" {
		t.Fatalf("refresh cookie = %+v; it must be HttpOnly, SameSite=Strict and sent only to the auth routes", refresh)
	}
	if strings.Contains(response.Body.String(), "refresh-1") || strings.Contains(response.Body.String(), "pbkdf2") {
		t.Fatal("the refresh token and password hash must never appear in a body")
	}
}

func TestRegistrationCanBeClosedAfterTheFirstAccount(t *testing.T) {
	backend := newIdentityBackend("")
	backend.hasUsers = true
	server := newTestServer(nil)
	server.backend, server.allowRegistration = backend, false
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	body := `{"email":"late@example.com","display_name":"Late","password":"a-long-enough-passphrase"}`
	if response := post(router, "/api/v1/auth/register", body, nil); response.Code != http.StatusForbidden || errorCode(t, response) != "REGISTRATION_CLOSED" {
		t.Fatalf("status %d body %s", response.Code, response.Body)
	}
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil))
	if !strings.Contains(status.Body.String(), `"registration_open":false`) {
		t.Fatalf("status = %s", status.Body)
	}
}

func TestRefreshNeedsTheCookieAndTheConsoleHeader(t *testing.T) {
	backend := newIdentityBackend(auth.RoleUser)
	handler := identityServer(backend)
	send := func(cookie string, header bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		if cookie != "" {
			request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: cookie})
		}
		if header {
			request.Header.Set(refreshHeader, "1")
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	if response := send("refresh-1", false); response.Code != http.StatusUnauthorized || len(backend.refreshed) != 1 || backend.refreshed[0] != "" {
		t.Fatalf("a cookie alone must not refresh (a cross-site form could send it): status %d, tokens tried %q", response.Code, backend.refreshed)
	}
	good := send("refresh-1", true)
	if good.Code != http.StatusOK || !strings.Contains(good.Body.String(), "access_token") {
		t.Fatalf("status %d body %s", good.Code, good.Body)
	}
	if rotated := cookieNamed(good, refreshCookieName); rotated == nil || rotated.Value != "refresh-2" {
		t.Fatalf("each refresh must replace the cookie, got %+v", rotated)
	}
	stale := send("refresh-1-replayed", true)
	if stale.Code != http.StatusUnauthorized {
		t.Fatalf("a token that was already used: status %d, want 401", stale.Code)
	}
	if cleared := cookieNamed(stale, refreshCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Fatal("a dead refresh cookie must be cleared so the console returns to sign-in")
	}
}

func TestLogoutEndsTheRefreshSession(t *testing.T) {
	backend := newIdentityBackend(auth.RoleUser)
	handler := identityServer(backend)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-1"})
	request.Header.Set(refreshHeader, "1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || len(backend.refreshed) != 1 || backend.refreshed[0] != "revoked:refresh-1" {
		t.Fatalf("status %d, revoked %v", recorder.Code, backend.refreshed)
	}
}

func TestLoginAnswersIdenticallyForUnknownAccountsAndWrongPasswords(t *testing.T) {
	hash, err := auth.HashPassword("a-long-enough-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	backend := newIdentityBackend(auth.RoleUser)
	backend.credentials = auth.Credentials{User: backend.user, PasswordHash: hash}
	handler := identityServer(backend)
	login := func(email, password string) *httptest.ResponseRecorder {
		return post(handler, "/api/v1/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, nil)
	}
	unknown, wrong := login("nobody@example.com", "a-long-enough-passphrase"), login("alice@example.com", "not-the-password-at-all")
	if unknown.Code != http.StatusUnauthorized || wrong.Code != http.StatusUnauthorized || errorCode(t, unknown) != errorCode(t, wrong) {
		t.Fatalf("unknown: %d %s, wrong password: %d %s", unknown.Code, unknown.Body, wrong.Code, wrong.Body)
	}
	if backend.sessions != 0 {
		t.Fatal("a failed sign-in must not open a session")
	}
	ok := login("ALICE@example.com", "a-long-enough-passphrase")
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), "access_token") || cookieNamed(ok, refreshCookieName) == nil {
		t.Fatalf("status %d body %s", ok.Code, ok.Body)
	}
}

func TestLoginAttemptsAreLimitedPerAccount(t *testing.T) {
	server := newTestServer(nil)
	server.backend = newIdentityBackend(auth.RoleUser)
	// Each attempt hashes a password, which is slow on a busy machine; a long window keeps the count from
	// resetting while the test runs.
	server.rateLimiter = newRequestRateLimiter(time.Hour)
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	var handler http.Handler = router
	var last *httptest.ResponseRecorder
	for range loginAttemptsPerEmail + 1 {
		last = post(handler, "/api/v1/auth/login", `{"email":"victim@example.com","password":"guess"}`, nil)
	}
	if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
		t.Fatalf("status %d; guessing one account's password must be throttled", last.Code)
	}
}
