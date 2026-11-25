package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

func TestLivenessReturnsRequestID(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	httpRequest.Header.Set(render.RequestIDHeader, "test-request-1")

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRequestID := responseRecorder.Header().Get(render.RequestIDHeader); responseRequestID != "test-request-1" {
		t.Fatalf("request ID = %q", responseRequestID)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := responseRecorder.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
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

func TestProductionMetricsRequireDatabaseAuthentication(t *testing.T) {
	configuration := config.Config{
		App: config.AppConfig{Production: true, ShutdownTimeout: time.Second},
		HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 1024, MaxConcurrentQueries: 4, MaxConcurrentLogQueries: 8, RequestTimeout: 20 * time.Second,
			RateLimitRequests: 120, RateLimitAuthRequests: 20, RateLimitReplayRequests: 10, RateLimitWindow: time.Minute},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
	}
	server := New(configuration, testDependencies())
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	server.httpServer.Handler.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusServiceUnavailable)
	}
}

func TestManagementTLSConfigurationUsesModernMinimum(t *testing.T) {
	server := New(config.Config{
		HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 1024, MaxConcurrentQueries: 4, MaxConcurrentLogQueries: 8, RequestTimeout: 20 * time.Second,
			RateLimitRequests: 120, RateLimitAuthRequests: 20, RateLimitReplayRequests: 10, RateLimitWindow: time.Minute},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
		App: config.AppConfig{
			ShutdownTimeout: time.Second,
			TLS:             config.TLSConfig{CertificateFile: "server.crt", PrivateKeyFile: "server.key"},
		},
	}, testDependencies())

	if server.httpServer.TLSConfig == nil || server.httpServer.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("management TLS config = %#v", server.httpServer.TLSConfig)
	}
}

func TestBackendServesNoConsole(t *testing.T) {
	// The console is a separate frontend; the backend only answers its API paths.
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if responseRecorder.Code != http.StatusNotFound {
		t.Fatalf("GET / = %d, want 404 now that the console is not embedded", responseRecorder.Code)
	}
}

func TestRateLimitReturnsStandardErrorAndRetryAfter(t *testing.T) {
	configuration := config.Config{
		HTTP:     config.HTTPConfig{ManagementMaxBodyBytes: 1024, RateLimitRequests: 1, RateLimitWindow: time.Minute},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
		App:      config.AppConfig{ShutdownTimeout: time.Second},
	}
	server := New(configuration, testDependencies())
	for attempt := 0; attempt < 2; attempt++ {
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
		request.RemoteAddr = "192.0.2.10:1234"
		server.httpServer.Handler.ServeHTTP(responseRecorder, request)
		if attempt == 0 && responseRecorder.Code == http.StatusTooManyRequests {
			t.Fatal("first request was rate limited")
		}
		if attempt == 1 {
			if responseRecorder.Code != http.StatusTooManyRequests || responseRecorder.Header().Get("Retry-After") == "" {
				t.Fatalf("status=%d retry-after=%q", responseRecorder.Code, responseRecorder.Header().Get("Retry-After"))
			}
			if !strings.Contains(responseRecorder.Body.String(), `"RATE_LIMITED"`) || responseRecorder.Header().Get(render.RequestIDHeader) == "" {
				t.Fatalf("unexpected rate limit response: %s", responseRecorder.Body.String())
			}
		}
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	for _, route := range allRoutes() {
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			httpServer := newTestServer(func(context.Context) error { return nil })
			httpServer.allowedOrigins["http://operator.example.test"] = struct{}{}
			responseRecorder := httptest.NewRecorder()
			httpRequest := httptest.NewRequest(http.MethodOptions, route.Path, nil)
			httpRequest.Header.Set("Origin", "http://operator.example.test")
			httpRequest.Header.Set("Access-Control-Request-Method", route.Method)

			httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

			if responseRecorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d", responseRecorder.Code)
			}
			if allowedOrigin := responseRecorder.Header().Get("Access-Control-Allow-Origin"); allowedOrigin != "http://operator.example.test" {
				t.Fatalf("allowed origin = %q", allowedOrigin)
			}
			if !middleware.AllowsCORSMethod(responseRecorder.Header().Get("Access-Control-Allow-Methods"), route.Method) {
				t.Fatalf("allowed methods do not contain %q", route.Method)
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

func TestCORSPreflightRejectsUnsupportedMethod(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	httpServer.allowedOrigins["http://operator.example.test"] = struct{}{}
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodOptions, "/api/v1/traces", nil)
	httpRequest.Header.Set("Origin", "http://operator.example.test")
	httpRequest.Header.Set("Access-Control-Request-Method", http.MethodConnect)

	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestMetricMethodBoundsCardinality(t *testing.T) {
	if normalizedMethod := middleware.MetricMethod("ATTACKER-CONTROLLED"); normalizedMethod != "OTHER" {
		t.Fatalf("method = %q", normalizedMethod)
	}
}

func newTestServer(readinessCheck func(context.Context) error) *Server {
	dependencies := testDependencies()
	dependencies.Readiness = readinessCheck
	return New(testConfig(), dependencies)
}

const testJWTSecret = "0123456789abcdef0123456789abcdef"

// testConfig is the configuration the tests start from.
func testConfig() config.Config {
	return config.Config{
		HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 1024, MaxConcurrentQueries: 4, MaxConcurrentLogQueries: 8, RequestTimeout: 20 * time.Second,
			RateLimitRequests: 120, RateLimitAuthRequests: 20, RateLimitReplayRequests: 10, RateLimitWindow: time.Minute},
		Auth:     config.AuthConfig{JWTSecret: testJWTSecret},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
		App:      config.AppConfig{ShutdownTimeout: time.Second},
	}
}

// testDependencies discard logs, pass the readiness check, and use a fresh metrics registry. Without
// a store the server builds every route but mounts none of them.
func testDependencies() Dependencies {
	return Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:   metrics.New("test", "test", "test"),
		Readiness: func(context.Context) error { return nil },
	}
}

// apiRouter mounts only the API routes, without the shared middleware.
func apiRouter(server *Server) *http.ServeMux {
	router := http.NewServeMux()
	server.RegisterAPIRoutes(router)
	return router
}
