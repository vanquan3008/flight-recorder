package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestRequestsNeedAValidAccessTokenAndAProject(t *testing.T) {
	handler := identityServer(newIdentityBackend(auth.RoleUser))
	if response := call(handler, http.MethodGet, "/api/v1/services", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: status %d", response.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	request.Header.Set("Authorization", "Bearer not-a-jwt")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("garbage token: status %d", recorder.Code)
	}

	// An access token signed long ago answers with a code the console reads as "refresh me".
	old, _, _ := auth.IssueAccessToken([]byte(testJWTSecret), "owner", time.Now().Add(-time.Hour))
	request = httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	request.Header.Set("Authorization", "Bearer "+old)
	request.Header.Set(projectHeader, testProjectIDForRoutes)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "TOKEN_EXPIRED" {
		t.Fatalf("expired token: status %d body %s", recorder.Code, recorder.Body)
	}

	token, _, _ := auth.IssueAccessToken([]byte(testJWTSecret), "owner", time.Now())
	request = httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "PROJECT_REQUIRED" {
		t.Fatalf("no project: status %d body %s", recorder.Code, recorder.Body)
	}
}

func TestTokensOfAnotherKindOrSecretDoNotOpenTheConsole(t *testing.T) {
	handler := identityServer(newIdentityBackend(auth.RoleOwner))
	now := time.Now()
	ingest, _ := auth.IssueIngestToken([]byte(testJWTSecret), testProjectIDForRoutes, "token-1", now, now.Add(time.Hour))
	foreign, _, _ := auth.IssueAccessToken([]byte("some-other-secret-some-other-secret"), "owner", now)
	for name, token := range map[string]string{"an ingest token": ingest, "another server's token": foreign} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set(projectHeader, testProjectIDForRoutes)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, recorder.Code)
		}
	}
}

func TestDisabledAccountsAndEndedTokensAreRefused(t *testing.T) {
	backend := newIdentityBackend(auth.RoleOwner)
	handler := identityServer(backend)
	disabled := time.Now()
	backend.user.DisabledAt = &disabled
	if response := call(handler, http.MethodGet, "/api/v1/services", "", "owner"); response.Code != http.StatusUnauthorized {
		t.Fatalf("a disabled account: status %d, want 401", response.Code)
	}
	backend.user.DisabledAt = nil
	// A password change ends access tokens issued before it.
	backend.user.TokensValidAfter = time.Now().Add(time.Minute)
	if response := call(handler, http.MethodGet, "/api/v1/services", "", "owner"); response.Code != http.StatusUnauthorized {
		t.Fatalf("a token from before the password change: status %d, want 401", response.Code)
	}
}

func TestOutsidersAreRefusedAndRolesDecideWhichRoutesRun(t *testing.T) {
	cases := []struct {
		role, method, path string
		want               int
	}{
		{"", http.MethodGet, "/api/v1/services", http.StatusForbidden},
		{auth.RoleUser, http.MethodGet, "/api/v1/services", http.StatusOK},
		{auth.RoleUser, http.MethodPost, "/api/v1/projects/" + testProjectIDForRoutes + "/ingest-tokens", http.StatusForbidden},
		{auth.RoleUser, http.MethodGet, "/api/v1/audit-events", http.StatusForbidden},
		{auth.RoleOwner, http.MethodPost, "/api/v1/projects/" + testProjectIDForRoutes + "/ingest-tokens", http.StatusCreated},
	}
	for _, testCase := range cases {
		handler := identityServer(newIdentityBackend(testCase.role))
		response := call(handler, testCase.method, testCase.path, `{"name":"checkout"}`, "owner")
		if response.Code != testCase.want {
			t.Errorf("%s %s as %q: status %d, want %d (body %s)", testCase.method, testCase.path, testCase.role, response.Code, testCase.want, response.Body)
		}
	}
}

func TestSuperAdministratorsReachEveryProjectAsOwner(t *testing.T) {
	backend := newIdentityBackend("")
	backend.user.PlatformAdmin = true
	handler := identityServer(backend)
	response := call(handler, http.MethodPost, "/api/v1/projects/"+testProjectIDForRoutes+"/ingest-tokens", `{"name":"checkout"}`, "owner")
	if response.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", response.Code, response.Body)
	}
}

func TestIngestTokensAreSignedForTheirProject(t *testing.T) {
	backend := newIdentityBackend(auth.RoleOwner)
	handler := identityServer(backend)
	response := call(handler, http.MethodPost, "/api/v1/projects/"+testProjectIDForRoutes+"/ingest-tokens", `{"name":"checkout"}`, "owner")
	var body struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.JWT == "" {
		t.Fatalf("status %d body %s", response.Code, response.Body)
	}
	claims, err := auth.VerifyJWT([]byte(testJWTSecret), body.JWT, auth.TokenIngest, time.Now())
	if err != nil || claims.Subject != testProjectIDForRoutes || claims.ID != "token-1" {
		t.Fatalf("claims = %+v, error = %v; the token must name this project and its row", claims, err)
	}
}

func TestIngestTokensUseDistinctIngestSecretWhenConfigured(t *testing.T) {
	backend := newIdentityBackend(auth.RoleOwner)
	distinctSecret := "custom-ingest-secret-000000000000000000"
	cfg := config.Config{
		HTTP:     config.HTTPConfig{ManagementMaxBodyBytes: 1024},
		Auth:     config.AuthConfig{JWTSecret: testJWTSecret, IngestJWTSecret: distinctSecret, AllowRegistration: true},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
		App:      config.AppConfig{ShutdownTimeout: time.Second},
	}
	server := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), backend, nil)
	router := http.NewServeMux()
	server.registerAPIRoutes(router)

	response := call(router, http.MethodPost, "/api/v1/projects/"+testProjectIDForRoutes+"/ingest-tokens", `{"name":"checkout"}`, "owner")
	var body struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.JWT == "" {
		t.Fatalf("status %d body %s", response.Code, response.Body)
	}
	if _, err := auth.VerifyJWT([]byte(testJWTSecret), body.JWT, auth.TokenIngest, time.Now()); err == nil {
		t.Fatal("expected verification with auth session secret to fail")
	}
	claims, err := auth.VerifyJWT([]byte(distinctSecret), body.JWT, auth.TokenIngest, time.Now())
	if err != nil || claims.Subject != testProjectIDForRoutes {
		t.Fatalf("claims = %+v, error = %v", claims, err)
	}
}

func TestPlatformRoutesNeedASuperAdministrator(t *testing.T) {
	backend := newIdentityBackend(auth.RoleOwner)
	handler := identityServer(backend)
	if response := call(handler, http.MethodGet, "/api/v1/users", "", "owner"); response.Code != http.StatusForbidden {
		t.Fatalf("a project owner: status %d, want 403", response.Code)
	}
	backend.user.PlatformAdmin = true
	if response := call(handler, http.MethodGet, "/api/v1/users", "", "owner"); response.Code != http.StatusOK {
		t.Fatalf("a super administrator: status %d body %s", response.Code, response.Body)
	}
}

func TestTemporaryPasswordBlocksEverythingButSessionRoutes(t *testing.T) {
	backend := newIdentityBackend(auth.RoleOwner)
	backend.user.MustChangePassword = true
	handler := identityServer(backend)
	blocked := call(handler, http.MethodGet, "/api/v1/services", "", "owner")
	if blocked.Code != http.StatusForbidden || errorCode(t, blocked) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("status %d body %s", blocked.Code, blocked.Body)
	}
	if me := call(handler, http.MethodGet, "/api/v1/auth/me", "", "owner"); me.Code != http.StatusOK {
		t.Fatalf("auth/me: status %d body %s", me.Code, me.Body)
	}
}

func TestAnyoneCanCreateAProjectButRegularAccountsAreLimited(t *testing.T) {
	backend := newIdentityBackend(auth.RoleUser)
	handler := identityServer(backend)
	response := call(handler, http.MethodPost, "/api/v1/projects", `{"slug":"mine","name":"Mine"}`, "owner")
	if response.Code != http.StatusCreated || backend.limit != auth.MaxOwnedProjects {
		t.Fatalf("status %d, limit %d; a regular account may create a project, within the limit", response.Code, backend.limit)
	}
	backend.projects = auth.MaxOwnedProjects
	if response = call(handler, http.MethodPost, "/api/v1/projects", `{"slug":"more","name":"More"}`, "owner"); response.Code != http.StatusConflict || errorCode(t, response) != "PROJECT_LIMIT" {
		t.Fatalf("over the limit: status %d body %s", response.Code, response.Body)
	}
	backend.user.PlatformAdmin = true
	call(handler, http.MethodPost, "/api/v1/projects", `{"slug":"admin","name":"Admin"}`, "owner")
	if backend.limit != 0 {
		t.Fatalf("a super administrator must not be limited, got %d", backend.limit)
	}
}
