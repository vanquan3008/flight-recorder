package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

// authOnlyBackend authenticates a key as the scope it names. Every other
// Backend method is nil, so a handler that runs would panic the test.
type authOnlyBackend struct{ Backend }

func (authOnlyBackend) AuthenticateAPIKey(_ context.Context, key string) (auth.Principal, error) {
	if key == "" {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	return auth.Principal{ProjectID: "project", Scopes: []string{key}}, nil
}

var pathParameter = regexp.MustCompile(`\{(\w+)\}`)

func TestRouterEnforcesEachRouteScopeBeforeItsHandler(t *testing.T) {
	server := newTestServer(nil)
	server.backend = authOnlyBackend{}
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	for _, route := range server.availableAPIRoutes() {
		path := pathParameter.ReplaceAllStringFunc(route.path, func(parameter string) string {
			if parameter == "{traceID}" {
				return strings.Repeat("ab", 16)
			}
			return "01900000-0000-7000-8000-000000000001"
		})
		for key, status := range map[string]int{"": http.StatusUnauthorized, "not-" + route.scope: http.StatusForbidden} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
			request.Header.Set(apiKeyHeader, key)
			router.ServeHTTP(recorder, request)
			if recorder.Code != status {
				t.Fatalf("%s %s with key %q: status %d, want %d", route.method, route.path, key, recorder.Code, status)
			}
		}
	}
}

// unavailableAuthBackend cannot verify any key. Its other methods are nil, so a
// handler that ran anyway would panic.
type unavailableAuthBackend struct{ Backend }

func (unavailableAuthBackend) AuthenticateAPIKey(context.Context, string) (auth.Principal, error) {
	return auth.Principal{}, errors.New("database unavailable")
}

func TestRoutesFailClosedWhenAuthorizationCannotBeVerified(t *testing.T) {
	server := newTestServer(nil)
	server.backend = unavailableAuthBackend{}
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	for _, route := range server.availableAPIRoutes() {
		path := pathParameter.ReplaceAllString(route.path, "01900000-0000-7000-8000-000000000001")
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
		request.Header.Set(apiKeyHeader, "admin")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "database unavailable") {
			t.Fatalf("%s %s: status %d body %s; an unverifiable key must be refused without leaking the cause", route.method, route.path, recorder.Code, recorder.Body.String())
		}
	}
}
