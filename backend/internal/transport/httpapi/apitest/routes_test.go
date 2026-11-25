package apitest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
)

const (
	testJWTSecret           = "0123456789abcdef0123456789abcdef"
	testProjectIDForRoutes  = "01900000-0000-7000-8000-0000000000c0"
	testPlatformAdminUserID = "platform"
)

// authOnlyBackend knows four people: an owner and a user of the project, an outsider who belongs to
// nothing, and a super administrator. Every other Backend method is nil, so a handler that runs
// without being expected to would panic the test.
type authOnlyBackend struct{ httpapi.Backend }

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
	request.Header.Set(api.HeaderProjectID, testProjectIDForRoutes)
}

var pathParameter = regexp.MustCompile(`\{(\w+)\}`)

type documentedRoute struct {
	Method, Path, Scope string
	Public              bool
}

// documentedRoutes lists the routes the server publishes in its OpenAPI document, which is generated
// from the same route list the router mounts.
func documentedRoutes(t *testing.T, server *httpapi.Server) []documentedRoute {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document struct {
		Paths map[string]map[string]struct {
			Scope string `json:"x-required-api-scope"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil || len(document.Paths) == 0 {
		t.Fatalf("OpenAPI document: %v %.200s", err, recorder.Body)
	}
	var routes []documentedRoute
	for path, operations := range document.Paths {
		for method, operation := range operations {
			routes = append(routes, documentedRoute{Method: strings.ToUpper(method), Path: path, Scope: operation.Scope, Public: operation.Scope == api.ScopePublic})
		}
	}
	return routes
}

func TestRouterEnforcesEachRouteScopeBeforeItsHandler(t *testing.T) {
	server := newTestServerWith(authOnlyBackend{})
	router := apiRouter(server)
	for _, route := range documentedRoutes(t, server) {
		if route.Public {
			continue // public routes authenticate the caller themselves; see identity_test.go
		}
		path := pathParameter.ReplaceAllStringFunc(route.Path, func(parameter string) string {
			if parameter == "{traceID}" {
				return strings.Repeat("ab", 16)
			}
			return testProjectIDForRoutes
		})
		call := func(who string) int {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(route.Method, path, strings.NewReader("{}"))
			signIn(request, who)
			router.ServeHTTP(recorder, request)
			return recorder.Code
		}
		if status := call(""); status != http.StatusUnauthorized {
			t.Fatalf("%s %s anonymous: status %d, want 401", route.Method, route.Path, status)
		}
		// Someone who belongs to nothing is refused by every project and platform route; a route open to any
		// signed-in person would run its handler, which the nil Backend turns into a panic, so it is skipped.
		if route.Scope != auth.ScopeSession {
			if status := call("outsider"); status != http.StatusForbidden {
				t.Fatalf("%s %s as an outsider: status %d, want 403", route.Method, route.Path, status)
			}
		}
		// A user may read and work incidents, but never run owner-only routes.
		if route.Scope == auth.ScopeAdmin || route.Scope == auth.ScopeReplay || route.Scope == auth.ScopePlatform {
			if status := call("user"); status != http.StatusForbidden {
				t.Fatalf("%s %s as a user: status %d, want 403", route.Method, route.Path, status)
			}
		}
	}
}

// unavailableAuthBackend cannot read accounts. Its other methods are nil, so a handler that ran
// anyway would panic.
type unavailableAuthBackend struct{ httpapi.Backend }

func (unavailableAuthBackend) GetUser(context.Context, string) (auth.User, error) {
	return auth.User{}, errors.New("database unavailable")
}

func TestRoutesFailClosedWhenAuthorizationCannotBeVerified(t *testing.T) {
	server := newTestServerWith(unavailableAuthBackend{})
	router := apiRouter(server)
	for _, route := range documentedRoutes(t, server) {
		if route.Public {
			continue
		}
		path := pathParameter.ReplaceAllString(route.Path, testProjectIDForRoutes)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.Method, path, strings.NewReader("{}"))
		signIn(request, "owner")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "database unavailable") {
			t.Fatalf("%s %s: status %d body %s; an unverifiable account must be refused without leaking the cause", route.Method, route.Path, recorder.Code, recorder.Body.String())
		}
	}
}

// stalledEnvironmentBackend never answers the environment lookup, like a pool
// with no free connection, and gives up only when its context ends.
type stalledEnvironmentBackend struct{ authOnlyBackend }

func (stalledEnvironmentBackend) ListEnvironments(ctx context.Context, _ string) ([]auth.Environment, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRequestDeadlineCoversAuthenticationLookups(t *testing.T) {
	router := apiRouter(newTestServerWith(stalledEnvironmentBackend{}, func(configuration *config.Config) {
		configuration.HTTP.RequestTimeout = 50 * time.Millisecond
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	signIn(request, "owner")
	request.Header.Set(api.HeaderEnvironment, "production")
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(recorder, request)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled environment lookup held the request past its deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 once the deadline ends the lookup", recorder.Code)
	}
}
