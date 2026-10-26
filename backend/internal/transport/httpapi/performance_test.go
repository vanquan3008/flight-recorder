package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
)

type performanceTestBackend struct {
	Backend
	project string
	filter  query.PerformanceFilter
	err     error
	calls   int
}

func (backend *performanceTestBackend) AuthenticateAPIKey(_ context.Context, key string) (auth.Principal, error) {
	if key == "" {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	return auth.Principal{ProjectID: "authenticated-project", Scopes: []string{key}}, nil
}

func (backend *performanceTestBackend) RequestPerformance(_ context.Context, project string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	backend.project, backend.filter = project, filter
	backend.calls++
	return query.RequestPerformance{From: filter.From, To: filter.To, Summary: query.PerformanceStats{Requests: 7}}, backend.err
}

func TestPerformanceEndpointScopesAndValidatesBeforeQuery(t *testing.T) {
	backend := &performanceTestBackend{}
	server := newTestServer(nil)
	server.backend, server.environmentID = backend, "configured-environment"
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	server.httpServer.Handler = router
	for _, test := range []struct {
		key, query string
		status     int
	}{
		{"", "", http.StatusUnauthorized},
		{"ingest", "", http.StatusForbidden},
		{"read", "?from=bad", http.StatusBadRequest},
		{"read", "?from=2026-01-01T00:00:00Z&to=2026-01-01T00:00:00Z", http.StatusBadRequest},
		{"read", "?from=2026-01-01T00:00:00Z&to=2026-02-02T00:00:00Z", http.StatusBadRequest},
		{"read", "?limit=0", http.StatusBadRequest},
		{"read", "?service=" + strings.Repeat("x", 4097), http.StatusBadRequest},
		{"read", "?project=untrusted&environment=untrusted&service=api&endpoint=GET%20%2Forders", http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/performance"+test.query, nil)
		request.Header.Set(apiKeyHeader, test.key)
		server.httpServer.Handler.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("key=%s query=%s: status=%d body=%s", test.key, test.query, recorder.Code, recorder.Body)
		}
	}
	if backend.calls != 1 || backend.project != "authenticated-project" || backend.filter.EnvironmentID != "configured-environment" || backend.filter.Service != "api" || backend.filter.Endpoint != "GET /orders" || backend.filter.To.Sub(backend.filter.From) != 15*time.Minute {
		t.Fatalf("scope/defaults/query count: %#v", backend)
	}
	backend.err = errors.New("sensitive SQL error")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/performance", nil)
	request.Header.Set(apiKeyHeader, "read")
	server.httpServer.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "sensitive") {
		t.Fatalf("query error exposed internal details: status=%d body=%s", recorder.Code, recorder.Body)
	}
}
