package access

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
)

const (
	testSecret    = "0123456789abcdef0123456789abcdef"
	testProjectID = "01900000-0000-7000-8000-0000000000c0"
)

// store knows the accounts in users and their roles in the one test project, which has a single
// environment named production.
type store struct {
	users map[string]auth.User
	roles map[string]string
}

func newStore() *store {
	return &store{
		users: map[string]auth.User{"owner": {ID: "owner"}, "user": {ID: "user"}},
		roles: map[string]string{"owner": auth.RoleOwner, "user": auth.RoleUser},
	}
}

func (store *store) GetUser(_ context.Context, id string) (auth.User, error) {
	if user, found := store.users[id]; found {
		return user, nil
	}
	return auth.User{}, auth.ErrUserNotFound
}

func (store *store) GetProject(context.Context, string) (auth.Project, error) {
	return auth.Project{ID: testProjectID, Slug: "project"}, nil
}

func (store *store) MemberRole(_ context.Context, _, userID string) (string, error) {
	return store.roles[userID], nil
}

func (store *store) ListEnvironments(context.Context, string) ([]auth.Environment, error) {
	return []auth.Environment{{ID: "environment", Name: "production"}}, nil
}

func testConfig() Config {
	return Config{TokenSecret: []byte(testSecret), MaxConcurrentQueries: 4, MaxConcurrentLogQueries: 8, RequestTimeout: 20 * time.Second}
}

func newGuard(store Store) *Guard {
	return New(testConfig(), store, nil, nil)
}

func signIn(request *http.Request, token string) {
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(api.HeaderProjectID, testProjectID)
}

func accessToken(t *testing.T, user auth.User) string {
	t.Helper()
	token, _, err := auth.IssueAccessTokenWithOptions([]byte(testSecret), user.ID, time.Now(), auth.AccessTokenTTL, "", "", user.TokenVersion())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestStreamRejectsAnExpiredTokenInsideClockSkew(t *testing.T) {
	guard := newGuard(newStore())
	now := time.Now()
	token, err := auth.SignJWT([]byte(testSecret), auth.Claims{Kind: auth.TokenAccess, Subject: "owner", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/live/stream", nil)
	signIn(request, token)
	response := httptest.NewRecorder()
	guard.streaming(auth.ScopeRead, func(http.ResponseWriter, *http.Request, auth.Principal) { t.Fatal("expired stream was admitted") })(response, request)
	if response.Code != http.StatusUnauthorized || rendertest.ErrorCode(t, response) != "TOKEN_EXPIRED" {
		t.Fatalf("expired stream: %d %s", response.Code, response.Body)
	}
}

func TestStreamAccessRechecksAccountAndMembership(t *testing.T) {
	now := time.Now()
	claims := auth.Claims{Subject: "user", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	principal := auth.Principal{UserID: "user", ProjectID: testProjectID}
	for _, tc := range []struct {
		name    string
		change  func(*auth.User, *string, *auth.Claims)
		allowed bool
	}{
		{"member", func(*auth.User, *string, *auth.Claims) {}, true},
		{"removed member", func(_ *auth.User, role *string, _ *auth.Claims) { *role = "" }, false},
		{"disabled account", func(user *auth.User, _ *string, _ *auth.Claims) { user.DisabledAt = &now }, false},
		{"changed password", func(user *auth.User, _ *string, _ *auth.Claims) { user.TokensValidAfter = now }, false},
		{"temporary password", func(user *auth.User, _ *string, _ *auth.Claims) { user.MustChangePassword = true }, false},
		{"expired token", func(_ *auth.User, _ *string, claims *auth.Claims) { claims.ExpiresAt = now.Add(-time.Second) }, false},
		{"administrator", func(user *auth.User, role *string, _ *auth.Claims) { *role = ""; user.PlatformAdmin = true }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore()
			user, role, checked := store.users["user"], store.roles["user"], claims
			tc.change(&user, &role, &checked)
			store.users["user"], store.roles["user"] = user, role
			if got := newGuard(store).streamAllowed(context.Background(), principal, checked); got != tc.allowed {
				t.Fatalf("stream access = %v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestStreamContextEndsWithTokenAndRequest(t *testing.T) {
	guard := newGuard(newStore())
	for _, expired := range []bool{false, true} {
		ctx, stop := context.WithCancel(context.Background())
		deadline := time.Now().Add(time.Hour)
		if expired {
			deadline = time.Now().Add(-time.Second)
		}
		streamCtx, cancel := guard.watchStream(ctx, auth.Principal{}, auth.Claims{ExpiresAt: deadline})
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

func TestQueryAndStreamRoutesAgreeOnTheRequestedEnvironment(t *testing.T) {
	store := newStore()
	guard := newGuard(store)
	selected := func(responseWriter http.ResponseWriter, _ *http.Request, principal auth.Principal) {
		responseWriter.Header().Set("X-Selected-Environment", principal.EnvironmentID)
	}
	routes := map[string]http.HandlerFunc{
		"query":  guard.Admit(api.Route{Scope: auth.ScopeRead, Handler: selected}),
		"logs":   guard.Admit(api.Route{Scope: auth.ScopeRead, Handler: selected, LogQuery: true}),
		"stream": guard.Admit(api.Route{Scope: auth.ScopeRead, Handler: selected, Streaming: true}),
	}
	token := accessToken(t, store.users["owner"])
	for name, route := range routes {
		for _, reference := range []string{"", "production", "environment"} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
			signIn(request, token)
			request.Header.Set(api.HeaderEnvironment, reference)
			response := httptest.NewRecorder()
			route(response, request)
			if response.Code != http.StatusOK || response.Header().Get("X-Selected-Environment") != "environment" {
				t.Fatalf("%s with environment %q: %d %s selected %q", name, reference, response.Code, response.Body, response.Header().Get("X-Selected-Environment"))
			}
		}
		request := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
		signIn(request, token)
		request.Header.Set(api.HeaderEnvironment, "staging")
		response := httptest.NewRecorder()
		route(response, request)
		if response.Code != http.StatusBadRequest || rendertest.ErrorCode(t, response) != "INVALID_ENVIRONMENT" {
			t.Fatalf("%s with an unknown environment: %d %s", name, response.Code, response.Body)
		}
	}
}

func TestRevocationWithinTheSameSecondAppliesToRESTAndStreams(t *testing.T) {
	store := newStore()
	guard := newGuard(store)
	user := store.users["user"]
	at := time.Now().UTC().Truncate(time.Second)
	user.TokensValidAfter = at.Add(100 * time.Millisecond)
	old := accessToken(t, user)
	user.TokensValidAfter = at.Add(200 * time.Millisecond)
	next := accessToken(t, user)
	store.users["user"] = user
	legacy, _, err := auth.IssueAccessToken([]byte(testSecret), user.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		allowed     bool
	}{
		{"revoked", old, false}, {"renewed", next, true}, {"legacy after revocation", legacy, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			request.Header.Set("Authorization", "Bearer "+tc.token)
			response := httptest.NewRecorder()
			if _, allowed := guard.Authenticate(response, request, auth.ScopeSession); allowed != tc.allowed {
				t.Fatalf("REST allowed=%t want=%t body=%s", allowed, tc.allowed, response.Body)
			}
			claims, err := auth.VerifyJWT([]byte(testSecret), tc.token, auth.TokenAccess, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			principal := auth.Principal{UserID: user.ID, ProjectID: testProjectID}
			if allowed := guard.streamAllowed(context.Background(), principal, claims); allowed != tc.allowed {
				t.Fatalf("stream allowed=%t want=%t", allowed, tc.allowed)
			}
		})
	}
}

func TestCachesAndPoolsFollowTheirConfiguration(t *testing.T) {
	guard := New(Config{
		IdentityCacheTTL: 2 * time.Minute, IdentityCacheLimit: 64,
		ScopeCacheTTL: 3 * time.Minute, ScopeCacheLimit: 32,
		MaxConcurrentQueries: 2, MaxConcurrentLogQueries: 5, RequestTimeout: time.Second,
	}, newStore(), nil, nil)
	if guard.projects.ttl != 2*time.Minute || guard.projects.limit != 64 || guard.roles.ttl != 2*time.Minute || guard.roles.limit != 64 {
		t.Fatalf("identity caches = (%s, %d) and (%s, %d)", guard.projects.ttl, guard.projects.limit, guard.roles.ttl, guard.roles.limit)
	}
	if guard.environments.ttl != 3*time.Minute || guard.environments.limit != 32 {
		t.Fatalf("scope cache = (%s, %d)", guard.environments.ttl, guard.environments.limit)
	}
	if cap(guard.queries.slots) != 2 || cap(guard.logQueries.slots) != 5 || cap(guard.signIns.slots) != signInSlots || guard.requestTimeout != time.Second {
		t.Fatalf("pools = %d/%d/%d, timeout %s", cap(guard.queries.slots), cap(guard.logQueries.slots), cap(guard.signIns.slots), guard.requestTimeout)
	}
}

func TestForgetClearsTheCachedEntries(t *testing.T) {
	guard := newGuard(newStore())
	now := time.Now()
	guard.projects.put(auth.Project{ID: testProjectID}, now, testProjectID)
	guard.roles.put(auth.RoleOwner, now, roleCacheKey(testProjectID, "owner"))
	guard.environments.put([]auth.Environment{{ID: "environment"}}, now, testProjectID)
	guard.ForgetProject(testProjectID)
	guard.ForgetRole(testProjectID, "owner")
	guard.ForgetEnvironments(testProjectID)
	if _, cached := guard.projects.get(testProjectID, now); cached {
		t.Fatal("project still cached")
	}
	if _, cached := guard.roles.get(roleCacheKey(testProjectID, "owner"), now); cached {
		t.Fatal("role still cached")
	}
	if _, cached := guard.environments.get(testProjectID, now); cached {
		t.Fatal("environments still cached")
	}
}
