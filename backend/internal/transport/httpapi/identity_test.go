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
	"github.com/vanquan3008/tracepilot/internal/query"
)

// identityBackend fakes the identity port. Its signed-in person is whoever the token names; the
// roles, flags and credentials are set per test.
type identityBackend struct {
	authOnlyBackend
	user        auth.User
	role        string
	credentials auth.Credentials
	hasUsers    bool
	registered  []auth.CreateUserParams
	refreshed   []string
	sessions    int
	tokensMade  []string
	projects    int
	limit       int
}

func (backend *identityBackend) GetUser(context.Context, string) (auth.User, error) {
	return backend.user, nil
}

func (backend *identityBackend) MemberRole(context.Context, string, string) (string, error) {
	return backend.role, nil
}

func (backend *identityBackend) ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error) {
	return []query.ServiceSummary{}, nil
}

func (backend *identityBackend) ListUsers(context.Context, auth.UserFilter) ([]auth.User, error) {
	return []auth.User{backend.user}, nil
}

func (backend *identityBackend) ListMemberships(context.Context, string) ([]auth.Membership, error) {
	return []auth.Membership{{Project: auth.Project{ID: testProjectIDForRoutes, Slug: "alpha"}, Role: backend.role}}, nil
}

func (backend *identityBackend) ListAllProjects(context.Context) ([]auth.Membership, error) {
	return []auth.Membership{{Project: auth.Project{ID: testProjectIDForRoutes, Slug: "alpha"}, Role: auth.RoleOwner}}, nil
}

func (backend *identityBackend) GetCredentials(_ context.Context, email string) (auth.Credentials, error) {
	if email != backend.credentials.User.Email {
		return auth.Credentials{}, auth.ErrUserNotFound
	}
	return backend.credentials, nil
}

func (backend *identityBackend) CreateSession(context.Context, string, string) (string, auth.Session, error) {
	backend.sessions++
	return "refresh-1", auth.Session{}, nil
}

func (backend *identityBackend) RefreshSession(_ context.Context, token string) (string, auth.User, error) {
	backend.refreshed = append(backend.refreshed, token)
	if token != "refresh-1" {
		return "", auth.User{}, auth.ErrSessionNotFound
	}
	return "refresh-2", backend.user, nil
}

func (backend *identityBackend) RevokeSession(_ context.Context, token string) error {
	backend.refreshed = append(backend.refreshed, "revoked:"+token)
	return nil
}

func (backend *identityBackend) RecordLogin(context.Context, string) error { return nil }

func (backend *identityBackend) HasUsers(context.Context) (bool, error) { return backend.hasUsers, nil }

func (backend *identityBackend) RegisterUser(_ context.Context, _ string, params auth.CreateUserParams, open bool) (auth.User, error) {
	if backend.hasUsers && !open {
		return auth.User{}, auth.ErrRegistrationClosed
	}
	backend.registered = append(backend.registered, params)
	user := auth.User{ID: "new-user", Email: params.Email, DisplayName: params.DisplayName, PlatformAdmin: !backend.hasUsers}
	backend.user = user
	return user, nil
}

func (backend *identityBackend) CreateProject(_ context.Context, params auth.CreateProjectParams, _ string, ownedLimit int, _ auth.Actor) (auth.Project, auth.Environment, error) {
	backend.limit = ownedLimit
	backend.projects++
	if ownedLimit > 0 && backend.projects > ownedLimit {
		return auth.Project{}, auth.Environment{}, auth.ErrTooManyProjects
	}
	return auth.Project{ID: "new-project", Slug: params.Slug}, auth.Environment{ID: "new-env", Name: "production"}, nil
}

func (backend *identityBackend) CreateIngestToken(_ context.Context, projectID string, _ auth.Actor, params auth.CreateIngestTokenParams, sign func(string, time.Time) (string, error)) (auth.IssuedIngestToken, error) {
	expires := time.Now().Add(24 * time.Hour)
	jwt, err := sign("token-1", expires)
	backend.tokensMade = append(backend.tokensMade, projectID)
	return auth.IssuedIngestToken{Token: auth.IngestToken{ID: "token-1", Name: params.Name, ExpiresAt: expires}, JWT: jwt}, err
}

func identityServer(backend *identityBackend) http.Handler {
	server := newTestServer(nil)
	server.backend = backend
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	return router
}

func newIdentityBackend(role string) *identityBackend {
	return &identityBackend{user: auth.User{ID: "owner", Email: "alice@example.com", DisplayName: "Alice"}, role: role}
}

func call(handler http.Handler, method, path, body, who string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	signIn(request, who)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response %q is not an error body: %v", recorder.Body.String(), err)
	}
	return body.Error.Code
}
