// Package apitest drives the assembled API, the router with its admission and every feature, against
// fake stores. The integration package runs the same API against real databases.
package apitest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

func testConfig() config.Config {
	return config.Config{
		HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 1024, MaxConcurrentQueries: 4, MaxConcurrentLogQueries: 8, RequestTimeout: 20 * time.Second,
			RateLimitRequests: 120, RateLimitAuthRequests: 20, RateLimitReplayRequests: 10, RateLimitWindow: time.Minute},
		Auth:     config.AuthConfig{JWTSecret: testJWTSecret},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
		App:      config.AppConfig{ShutdownTimeout: time.Second},
	}
}

// testDependencies discard logs, pass the readiness check, and use a fresh metrics registry.
func testDependencies(store httpapi.Backend) httpapi.Dependencies {
	return httpapi.Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:   metrics.New("test", "test", "test"),
		Readiness: func(context.Context) error { return nil },
		Store:     store,
	}
}

// newTestServerWith serves store with testConfig changed by adjust.
func newTestServerWith(store httpapi.Backend, adjust ...func(*config.Config)) *httpapi.Server {
	configuration := testConfig()
	for _, change := range adjust {
		change(&configuration)
	}
	return httpapi.New(configuration, testDependencies(store))
}

// apiRouter mounts only the API routes, without the shared middleware and its rate limits.
func apiRouter(server *httpapi.Server) *http.ServeMux {
	router := http.NewServeMux()
	server.RegisterAPIRoutes(router)
	return router
}
