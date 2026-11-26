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
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/access"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/analysis"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/incidents"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/ingesttokens"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/logs"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/members"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/projects"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/session"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/traces"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/users"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/webhooks"
)

// backend lets an API test share one fake across the features it exercises.
type backend interface {
	access.Store
	session.Backend
	users.Backend
	projects.Backend
	projects.RetentionBackend
	members.Backend
	ingesttokens.Backend
	traces.Backend
	logs.Backend
	analysis.Backend
	incidents.Backend
}

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
func testDependencies(store backend) httpapi.Dependencies {
	insights, _ := store.(analysis.InsightsBackend)
	performance, _ := store.(analysis.PerformanceBackend)
	incidentAnalysis, _ := store.(incidents.AnalysisBackend)
	webhookStore, _ := store.(webhooks.Backend)
	return httpapi.Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:   metrics.New("test", "test", "test"),
		Readiness: func(context.Context) error { return nil },
		Backends: httpapi.Backends{
			Access: store, Session: store, Users: store, Projects: store, ProjectRetention: store, Members: store,
			IngestTokens: store, Traces: store, Logs: store, Services: store, Incidents: store,
			Insights: insights, Performance: performance, IncidentAnalysis: incidentAnalysis, Webhooks: webhookStore,
		},
	}
}

// newTestServerWith serves store with testConfig changed by adjust.
func newTestServerWith(store backend, adjust ...func(*config.Config)) *httpapi.Server {
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
