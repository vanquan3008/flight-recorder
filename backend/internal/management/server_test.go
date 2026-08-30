package management

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/metrics"
)

func TestLivenessReturnsRequestID(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	httpRequest.Header.Set(requestIDHeader, "test-request-1")

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRequestID := responseRecorder.Header().Get(requestIDHeader); responseRequestID != "test-request-1" {
		t.Fatalf("request ID = %q", responseRequestID)
	}
}

func TestCORSAllowsConfiguredOriginOnLiveness(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	httpServer.allowedOrigins["http://operator.example.test"] = struct{}{}
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	httpRequest.Header.Set("Origin", "http://operator.example.test")

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if allowedOrigin := responseRecorder.Header().Get("Access-Control-Allow-Origin"); allowedOrigin != "http://operator.example.test" {
		t.Fatalf("allowed origin = %q", allowedOrigin)
	}
}

func TestReadinessReflectsState(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestReadinessChecksDatabase(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return errors.New("database unavailable") })
	httpServer.ready.Store(true)
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestMetricsAreExposed(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestBackendDoesNotServeDashboard(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	tests := []struct {
		path   string
		method string
	}{
		{path: "/api/v1/traces", method: http.MethodGet},
		{path: "/api/v1/incidents/01900000-0000-7000-8000-000000000001", method: http.MethodPatch},
		{path: "/api/v1/api-keys/01900000-0000-7000-8000-000000000001", method: http.MethodDelete},
		{path: "/health/ready", method: http.MethodGet},
	}

	for _, preflight := range tests {
		t.Run(preflight.method+" "+preflight.path, func(t *testing.T) {
			httpServer := newTestServer(func(context.Context) error { return nil })
			httpServer.allowedOrigins["http://operator.example.test"] = struct{}{}
			responseRecorder := httptest.NewRecorder()
			httpRequest := httptest.NewRequest(http.MethodOptions, preflight.path, nil)
			httpRequest.Header.Set("Origin", "http://operator.example.test")
			httpRequest.Header.Set("Access-Control-Request-Method", preflight.method)

			httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

			if responseRecorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d", responseRecorder.Code)
			}
			if allowedOrigin := responseRecorder.Header().Get("Access-Control-Allow-Origin"); allowedOrigin != "http://operator.example.test" {
				t.Fatalf("allowed origin = %q", allowedOrigin)
			}
			if allowedMethods := responseRecorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(allowedMethods, preflight.method) {
				t.Fatalf("allowed methods %q do not contain %q", allowedMethods, preflight.method)
			}
		})
	}
}

func TestCORSPreflightRejectsUnknownOrigin(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodOptions, "/api/v1/traces", nil)
	httpRequest.Header.Set("Origin", "https://untrusted.example.test")

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestMetricMethodBoundsCardinality(t *testing.T) {
	if normalizedMethod := metricMethod("ATTACKER-CONTROLLED"); normalizedMethod != "OTHER" {
		t.Fatalf("method = %q", normalizedMethod)
	}
}

func newTestServer(readinessCheck func(context.Context) error) *Server {
	configuration := config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		ShutdownTimeout: time.Second,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(configuration, logger, readinessCheck, metrics.New("test", "test", "test"), nil, nil)
}
