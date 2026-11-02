package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

type performanceTestBackend struct {
	authOnlyBackend
	project string
	filter  query.PerformanceFilter
	err     error
	calls   int
}

func (backend *performanceTestBackend) RequestPerformance(_ context.Context, project string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	backend.project, backend.filter = project, filter
	backend.calls++
	return query.RequestPerformance{From: filter.From, To: filter.To, Summary: query.PerformanceStats{Requests: 7}}, backend.err
}

func TestPerformanceEndpointScopesAndValidatesBeforeQuery(t *testing.T) {
	backend := &performanceTestBackend{}
	server := newTestServer(nil)
	server.backend, server.environmentID, server.defaultProjectID = backend, "configured-environment", testProjectIDForRoutes
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	server.httpServer.Handler = router
	for _, test := range []struct {
		key, query string
		status     int
	}{
		{"", "", http.StatusUnauthorized},
		{"outsider", "", http.StatusForbidden},
		{"user", "?from=bad", http.StatusBadRequest},
		{"user", "?from=2026-01-01T00:00:00Z&to=2026-01-01T00:00:00Z", http.StatusBadRequest},
		{"user", "?from=2026-01-01T00:00:00Z&to=2026-02-02T00:00:00Z", http.StatusBadRequest},
		{"user", "?limit=0", http.StatusBadRequest},
		{"user", "?service=" + strings.Repeat("x", 4097), http.StatusBadRequest},
		{"user", "?project=untrusted&environment=untrusted&service=api&endpoint=GET%20%2Forders", http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/performance"+test.query, nil)
		signIn(request, test.key)
		server.httpServer.Handler.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("user=%s query=%s: status=%d body=%s", test.key, test.query, recorder.Code, recorder.Body)
		}
	}
	if backend.calls != 1 || backend.project != testProjectIDForRoutes || backend.filter.EnvironmentID != "configured-environment" || backend.filter.Service != "api" || backend.filter.Endpoint != "GET /orders" || backend.filter.To.Sub(backend.filter.From) != 15*time.Minute {
		t.Fatalf("scope/defaults/query count: %#v", backend)
	}
	backend.err = errors.New("sensitive SQL error")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/performance", nil)
	signIn(request, "user")
	server.httpServer.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "sensitive") {
		t.Fatalf("query error exposed internal details: status=%d body=%s", recorder.Code, recorder.Body)
	}
}
