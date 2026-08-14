package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/config"
	flightmetrics "github.com/flight-observability/flight/internal/metrics"
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
	return New(configuration, logger, readinessCheck, flightmetrics.New("test", "test", "test"))
}
