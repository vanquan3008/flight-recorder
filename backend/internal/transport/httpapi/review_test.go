package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func (backend *identityBackend) RemoveMember(_ context.Context, _, userID string, _ auth.Actor, allowed func(string) bool) error {
	if allowed != nil && !allowed(backend.role) {
		return auth.ErrForbiddenRoleChange
	}
	backend.refreshed = append(backend.refreshed, "removed:"+userID)
	return nil
}

func TestTemporaryPasswordBlocksSessionAndPlatformRoutesToo(t *testing.T) {
	backend := newIdentityBackend(auth.RoleOwner)
	backend.user.MustChangePassword = true
	backend.user.PlatformAdmin = true
	handler := identityServer(backend)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/projects"}, // open to any signed-in person
		{http.MethodGet, "/api/v1/projects"},  // likewise
		{http.MethodGet, "/api/v1/users"},     // super administrators only
		{http.MethodPost, "/api/v1/users"},    // likewise
		{http.MethodGet, "/api/v1/services"},  // project route
	} {
		response := call(handler, route.method, route.path, `{"slug":"x","name":"X"}`, "owner")
		if response.Code != http.StatusForbidden || errorCode(t, response) != "PASSWORD_CHANGE_REQUIRED" {
			t.Errorf("%s %s on a temporary password: status %d body %s", route.method, route.path, response.Code, response.Body)
		}
	}
	if response := call(handler, http.MethodGet, "/api/v1/auth/me", "", "owner"); response.Code != http.StatusOK {
		t.Fatalf("the account may still look at itself: status %d", response.Code)
	}
}

func TestAnyMemberMayLeaveButOnlyOwnersRemoveOthers(t *testing.T) {
	userID := "01900000-0000-7000-8000-0000000000b9"
	cases := []struct {
		name, role, target string
		want               int
	}{
		{"a user leaving", auth.RoleUser, "owner", http.StatusNoContent}, // the signed-in account is "owner" in the fake
		{"a user removing someone else", auth.RoleUser, userID, http.StatusForbidden},
		{"an owner removing someone else", auth.RoleOwner, userID, http.StatusNoContent},
	}
	for _, testCase := range cases {
		backend := newIdentityBackend(testCase.role)
		backend.user.ID = "owner"
		handler := identityServer(backend)
		target := testCase.target
		if target == "owner" {
			target = userID
			backend.user.ID = userID // the person signing in is the one being removed
		}
		response := call(handler, http.MethodDelete, "/api/v1/projects/"+testProjectIDForRoutes+"/members/"+target, "", "owner")
		if response.Code != testCase.want {
			t.Errorf("%s: status %d, want %d (body %s)", testCase.name, response.Code, testCase.want, response.Body)
		}
	}
}

func metricsServer(token string) *Server {
	server := newTestServer(nil)
	server.backend = authOnlyBackend{}
	server.metricsRequireAuth = true
	server.metricsToken = token
	return server
}

func scrape(server *Server, who, bearer string) int {
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if who != "" {
		signIn(request, who)
	} else if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestMetricsInProductionNeedAScrapeTokenOrASuperAdministrator(t *testing.T) {
	token := "scrape-token-0123456789abcdef0123456789"
	server := metricsServer(token)
	if status := scrape(server, "", token); status != http.StatusOK {
		t.Fatalf("the scrape token: status %d, want 200", status)
	}
	for name, status := range map[string]int{
		"no credential":   scrape(server, "", ""),
		"a wrong token":   scrape(server, "", token+"x"),
		"a project owner": scrape(server, "owner", ""),
		"a plain user":    scrape(server, "user", ""),
	} {
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("%s: status %d, want a refusal", name, status)
		}
	}
	if status := scrape(server, testPlatformAdminUserID, ""); status != http.StatusOK {
		t.Fatalf("a signed-in super administrator: status %d, want 200", status)
	}
	// Without a configured token the scrape token path does not exist, not even for an empty bearer.
	if status := scrape(metricsServer(""), "", ""); status != http.StatusUnauthorized {
		t.Fatalf("no token configured and no credential: status %d, want 401", status)
	}
}

func TestUnknownMemberRoleIsABadRequestNotAForbiddenOne(t *testing.T) {
	handler := identityServer(newIdentityBackend(auth.RoleOwner))
	member := "01900000-0000-7000-8000-0000000000b9"
	for _, role := range []string{"admin", "viewer", ""} {
		response := call(handler, http.MethodPatch, "/api/v1/projects/"+testProjectIDForRoutes+"/members/"+member, `{"role":"`+role+`"}`, "owner")
		if response.Code != http.StatusBadRequest || errorCode(t, response) != "INVALID_REQUEST" {
			t.Errorf("role %q: status %d body %s, want 400 INVALID_REQUEST", role, response.Code, response.Body)
		}
	}
	response := call(handler, http.MethodPost, "/api/v1/projects/"+testProjectIDForRoutes+"/members", `{"email":"a@example.com","role":"admin"}`, "owner")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "owner, user") {
		t.Errorf("adding with an unknown role: status %d body %s, want 400 naming owner and user", response.Code, response.Body)
	}
}

func TestSignInDoesNotKeepAnOversizedEmailAsARateLimitKey(t *testing.T) {
	server := newTestServer(nil)
	server.backend = newIdentityBackend(auth.RoleOwner)
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	body := `{"email":"` + strings.Repeat("a", 100_000) + `@example.com","password":"a-long-enough-passphrase"}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)))
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "INVALID_CREDENTIALS" {
		t.Fatalf("status %d body %.200s, want 401 INVALID_CREDENTIALS", recorder.Code, recorder.Body)
	}
	server.rateLimiter.mu.Lock()
	defer server.rateLimiter.mu.Unlock()
	if len(server.rateLimiter.clients) == 0 {
		t.Fatal("the attempt was not counted")
	}
	for key := range server.rateLimiter.clients {
		if len(key) > len("login:")+maxEmailLength {
			t.Fatalf("a rate-limit key of %d bytes was stored", len(key))
		}
	}
}
