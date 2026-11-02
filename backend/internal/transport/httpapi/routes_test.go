package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

const (
	testJWTSecret           = "0123456789abcdef0123456789abcdef"
	testProjectIDForRoutes  = "01900000-0000-7000-8000-0000000000c0"
	testPlatformAdminUserID = "platform"
)

// authOnlyBackend knows four people: an owner and a user of the project, an outsider who belongs to
// nothing, and a super administrator. Every other Backend method is nil, so a handler that runs
// without being expected to would panic the test.
type authOnlyBackend struct{ Backend }

func (authOnlyBackend) GetUser(_ context.Context, id string) (auth.User, error) {
	switch id {
	case "owner", "user", "outsider":
		return auth.User{ID: id}, nil
	case testPlatformAdminUserID:
		return auth.User{ID: id, PlatformAdmin: true}, nil
	}
	return auth.User{}, auth.ErrUserNotFound
}

func (authOnlyBackend) GetProject(_ context.Context, reference string) (auth.Project, error) {
	return auth.Project{ID: testProjectIDForRoutes, Slug: "project", Name: "Project"}, nil
}

func (authOnlyBackend) MemberRole(_ context.Context, _, userID string) (string, error) {
	switch userID {
	case "owner":
		return auth.RoleOwner, nil
	case "user":
		return auth.RoleUser, nil
	}
	return "", nil
}

// ListEnvironments gives every project one environment, so handlers that read the request's
// environment run against fakes without a database.
func (authOnlyBackend) ListEnvironments(context.Context, string) ([]auth.Environment, error) {
	return []auth.Environment{{ID: "environment", Name: "production"}}, nil
}

// signIn authenticates a request as one of the people authOnlyBackend knows and points it at the
// test project. An empty name leaves the request anonymous.
func signIn(request *http.Request, who string) {
	if who == "" {
		return
	}
	token, _, err := auth.IssueAccessToken([]byte(testJWTSecret), who, time.Now())
	if err != nil {
		panic(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(projectHeader, testProjectIDForRoutes)
}

var pathParameter = regexp.MustCompile(`\{(\w+)\}`)

func TestRouterEnforcesEachRouteScopeBeforeItsHandler(t *testing.T) {
	server := newTestServer(nil)
	server.backend = authOnlyBackend{}
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	for _, route := range server.availableAPIRoutes() {
		if route.public {
			continue // public routes authenticate the caller themselves; see identity_test.go
		}
		path := pathParameter.ReplaceAllStringFunc(route.path, func(parameter string) string {
			if parameter == "{traceID}" {
				return strings.Repeat("ab", 16)
			}
			return testProjectIDForRoutes
		})
		call := func(who string) int {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
			signIn(request, who)
			router.ServeHTTP(recorder, request)
			return recorder.Code
		}
		if status := call(""); status != http.StatusUnauthorized {
			t.Fatalf("%s %s anonymous: status %d, want 401", route.method, route.path, status)
		}
		// Someone who belongs to nothing is refused by every project and platform route; a route open to any
		// signed-in person would run its handler, which the nil Backend turns into a panic, so it is skipped.
		if route.scope != auth.ScopeSession {
			if status := call("outsider"); status != http.StatusForbidden {
				t.Fatalf("%s %s as an outsider: status %d, want 403", route.method, route.path, status)
			}
		}
		// A user may read and work incidents, but never run owner-only routes.
		if route.scope == auth.ScopeAdmin || route.scope == auth.ScopeReplay || route.scope == auth.ScopePlatform {
			if status := call("user"); status != http.StatusForbidden {
				t.Fatalf("%s %s as a user: status %d, want 403", route.method, route.path, status)
			}
		}
	}
}

// unavailableAuthBackend cannot read accounts. Its other methods are nil, so a handler that ran
// anyway would panic.
type unavailableAuthBackend struct{ Backend }

func (unavailableAuthBackend) GetUser(context.Context, string) (auth.User, error) {
	return auth.User{}, errors.New("database unavailable")
}

func TestRoutesFailClosedWhenAuthorizationCannotBeVerified(t *testing.T) {
	server := newTestServer(nil)
	server.backend = unavailableAuthBackend{}
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	for _, route := range server.availableAPIRoutes() {
		if route.public {
			continue
		}
		path := pathParameter.ReplaceAllString(route.path, testProjectIDForRoutes)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
		signIn(request, "owner")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "database unavailable") {
			t.Fatalf("%s %s: status %d body %s; an unverifiable account must be refused without leaking the cause", route.method, route.path, recorder.Code, recorder.Body.String())
		}
	}
}
