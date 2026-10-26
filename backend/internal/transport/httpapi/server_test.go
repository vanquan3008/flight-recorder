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
		Production:      true,
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		ShutdownTimeout: time.Second,
	}
	server := New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), nil, nil)
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	server.httpServer.Handler.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusServiceUnavailable)
	}
}

func TestManagementTLSConfigurationUsesModernMinimum(t *testing.T) {
	server := New(config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		TLS:             config.TLSConfig{CertificateFile: "server.crt", PrivateKeyFile: "server.key"},
		ShutdownTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), nil, nil)

	if server.httpServer.TLSConfig == nil || server.httpServer.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("management TLS config = %#v", server.httpServer.TLSConfig)
	}
}

func TestBackendServesOperatorConsole(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusOK || !strings.Contains(responseRecorder.Body.String(), "TracePilot Console") {
		t.Fatalf("unexpected dashboard response: %d %s", responseRecorder.Code, responseRecorder.Body.String())
	}
}

func TestRateLimitReturnsStandardErrorAndRetryAfter(t *testing.T) {
	configuration := config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024, RateLimitRequests: 1, RateLimitWindow: time.Minute},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		ShutdownTimeout: time.Second,
	}
	server := New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), nil, nil)
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
			if !strings.Contains(responseRecorder.Body.String(), `"RATE_LIMITED"`) || responseRecorder.Header().Get(requestIDHeader) == "" {
				t.Fatalf("unexpected rate limit response: %s", responseRecorder.Body.String())
			}
		}
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	for _, route := range apiRouteDefinitions {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			httpServer := newTestServer(func(context.Context) error { return nil })
			httpServer.allowedOrigins["http://operator.example.test"] = struct{}{}
			responseRecorder := httptest.NewRecorder()
			httpRequest := httptest.NewRequest(http.MethodOptions, route.path, nil)
			httpRequest.Header.Set("Origin", "http://operator.example.test")
			httpRequest.Header.Set("Access-Control-Request-Method", route.method)

			httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)

			if responseRecorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d", responseRecorder.Code)
			}
			if allowedOrigin := responseRecorder.Header().Get("Access-Control-Allow-Origin"); allowedOrigin != "http://operator.example.test" {
				t.Fatalf("allowed origin = %q", allowedOrigin)
			}
			if !allowsCORSMethod(responseRecorder.Header().Get("Access-Control-Allow-Methods"), route.method) {
				t.Fatalf("allowed methods do not contain %q", route.method)
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
