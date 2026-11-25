// Package health serves liveness, readiness, and the Prometheus metrics endpoint.
package health

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// Authenticator signs in the caller of /metrics when it is not a scraper.
type Authenticator interface {
	Authenticate(http.ResponseWriter, *http.Request, string) (auth.Principal, bool)
}

type Config struct {
	ReadinessTimeout time.Duration
	// MetricsToken lets a scraper read /metrics without signing in.
	MetricsToken string
	// RequireMetricsAuth closes /metrics to everyone but scrapers and the super administrator.
	RequireMetricsAuth bool
}

type Handler struct {
	config        Config
	ready         func() bool
	check         func(context.Context) error
	metrics       *metrics.Registry
	scrape        http.Handler
	authenticator Authenticator
}

// New reports ready while ready returns true and check succeeds. authenticator may be nil, and then
// a closed /metrics answers 503.
func New(configuration Config, ready func() bool, check func(context.Context) error, registry *metrics.Registry, authenticator Authenticator) *Handler {
	return &Handler{config: configuration, ready: ready, check: check, metrics: registry, scrape: registry.Handler(), authenticator: authenticator}
}

// Register mounts the endpoints. They sit outside the API route table and its admission.
func (handler *Handler) Register(router *http.ServeMux) {
	router.HandleFunc("GET /health/live", handler.live)
	router.HandleFunc("GET /health/ready", handler.readiness)
	router.HandleFunc("GET /metrics", handler.metricsEndpoint)
}

func (handler *Handler) live(responseWriter http.ResponseWriter, _ *http.Request) {
	render.WriteJSON(responseWriter, http.StatusOK, map[string]string{"status": "ok"})
}

func (handler *Handler) readiness(responseWriter http.ResponseWriter, request *http.Request) {
	if !handler.ready() {
		handler.metrics.SetDatabaseReady(false)
		render.WriteJSON(responseWriter, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	readinessContext, cancelReadiness := context.WithTimeout(request.Context(), handler.config.ReadinessTimeout)
	defer cancelReadiness()
	if readinessError := handler.check(readinessContext); readinessError != nil {
		handler.metrics.SetDatabaseReady(false)
		render.WriteJSON(responseWriter, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	handler.metrics.SetDatabaseReady(true)
	render.WriteJSON(responseWriter, http.StatusOK, map[string]string{"status": "ready"})
}

func (handler *Handler) metricsEndpoint(responseWriter http.ResponseWriter, request *http.Request) {
	if !handler.config.RequireMetricsAuth {
		handler.scrape.ServeHTTP(responseWriter, request)
		return
	}
	if handler.authenticator == nil {
		render.WriteAPIError(responseWriter, http.StatusServiceUnavailable, "UNAVAILABLE", "metrics authentication is unavailable")
		return
	}
	// A scraper cannot sign in and renew a 15 minute token, so it presents the configured scrape token.
	// Anyone else needs to be a signed-in super administrator.
	token := api.BearerToken(request)
	if handler.config.MetricsToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(handler.config.MetricsToken)) == 1 {
		handler.scrape.ServeHTTP(responseWriter, request)
		return
	}
	if _, ok := handler.authenticator.Authenticate(responseWriter, request, auth.ScopePlatform); !ok {
		return
	}
	handler.scrape.ServeHTTP(responseWriter, request)
}
