// Package httpapi serves the console's REST API, the OpenAPI document, health checks, and /metrics.
// It assembles the feature packages into one router behind the shared middleware.
package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/live"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/access"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/health"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/incidents"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/ingesttokens"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
)

// Dependencies are the collaborators cmd/tracepilot creates and closes.
type Dependencies struct {
	Logger    *slog.Logger
	Metrics   *metrics.Registry
	Readiness func(context.Context) error
	// Backends serves the API. Without an access store only health checks, /metrics, and OpenAPI are mounted.
	Backends Backends
	// Replay runs captured requests. Without it the replay route is left out.
	Replay incidents.ReplayRunner
	// TokenInvalidator drops revoked ingest tokens from the ingest cache at once.
	TokenInvalidator ingesttokens.Invalidator
	// StreamHub feeds the live stream and is closed on shutdown.
	StreamHub *live.Hub
}

// Server owns the router, the middleware chain, and the admission guard. It does not own the store,
// the metrics registry, or the stream hub.
type Server struct {
	httpServer      *http.Server
	logger          *slog.Logger
	metrics         *metrics.Registry
	ready           atomic.Bool
	shutdownTimeout time.Duration
	streamHub       *live.Hub
	tlsCertificate  string
	tlsPrivateKey   string

	guard  *access.Guard
	routes []api.Route

	rateLimiter    *middleware.RequestRateLimiter
	rateLimits     rateLimits
	trustedProxies []netip.Prefix
	allowedOrigins map[string]struct{}
	maxBodyBytes   int64
}

// rateLimits are the requests a client may send per window in each class.
type rateLimits struct {
	queries int
	auth    int
	replay  int
}

func New(configuration config.Config, dependencies Dependencies) *Server {
	configuration.HTTP = configuration.HTTP.WithRequestDefaults()
	server := &Server{
		logger:          dependencies.Logger,
		metrics:         dependencies.Metrics,
		shutdownTimeout: configuration.App.ShutdownTimeout,
		streamHub:       dependencies.StreamHub,
		tlsCertificate:  configuration.App.TLS.CertificateFile,
		tlsPrivateKey:   configuration.App.TLS.PrivateKeyFile,

		rateLimiter: middleware.NewRequestRateLimiter(configuration.HTTP.RateLimitWindow),
		rateLimits: rateLimits{
			queries: configuration.HTTP.RateLimitRequests,
			auth:    configuration.HTTP.RateLimitAuthRequests,
			replay:  configuration.HTTP.RateLimitReplayRequests,
		},
		trustedProxies: configuration.HTTP.TrustedProxyPrefixes(),
		allowedOrigins: make(map[string]struct{}, len(configuration.HTTP.AllowedOrigins)),
		maxBodyBytes:   configuration.HTTP.ManagementMaxBodyBytes,
	}
	for _, origin := range configuration.HTTP.AllowedOrigins {
		server.allowedOrigins[origin] = struct{}{}
	}
	server.guard = access.New(access.Config{
		TokenSecret:             []byte(configuration.Auth.JWTSecret),
		TokenIssuer:             configuration.Auth.EffectiveIssuer(),
		TokenAudience:           configuration.Auth.EffectiveAudience(),
		DefaultProjectID:        configuration.Ingestion.ProjectID,
		DefaultEnvironmentID:    configuration.Ingestion.EnvironmentID,
		IdentityCacheTTL:        configuration.Cache.EffectiveIdentityTTL(),
		IdentityCacheLimit:      configuration.Cache.EffectiveIdentityLimit(),
		ScopeCacheTTL:           configuration.Cache.EffectiveScopeTTL(),
		ScopeCacheLimit:         configuration.Cache.EffectiveScopeLimit(),
		MaxConcurrentQueries:    configuration.HTTP.MaxConcurrentQueries,
		MaxConcurrentLogQueries: configuration.HTTP.MaxConcurrentLogQueries,
		RequestTimeout:          configuration.HTTP.RequestTimeout,
	}, dependencies.Backends.Access, dependencies.Logger, dependencies.Metrics)
	server.routes = server.featureRoutes(configuration, dependencies)

	router := http.NewServeMux()
	router.HandleFunc("GET /openapi.json", server.openAPI)
	var authenticator health.Authenticator
	if dependencies.Backends.Access != nil {
		authenticator = server.guard
		server.RegisterAPIRoutes(router)
	}
	health.New(health.Config{
		ReadinessTimeout:   configuration.Database.ReadinessTimeout,
		MetricsToken:       configuration.HTTP.MetricsToken,
		RequireMetricsAuth: configuration.App.Production,
	}, server.ready.Load, dependencies.Readiness, dependencies.Metrics, authenticator).Register(router)

	server.httpServer = &http.Server{
		Addr:              configuration.HTTP.Address,
		Handler:           server.withMiddleware(router, corsMethods(server.routes)),
		ReadHeaderTimeout: configuration.HTTP.ReadHeaderTimeout,
		ReadTimeout:       configuration.HTTP.ReadTimeout,
		WriteTimeout:      configuration.HTTP.WriteTimeout,
		IdleTimeout:       configuration.HTTP.IdleTimeout,
		MaxHeaderBytes:    configuration.HTTP.MaxHeaderBytes,
	}
	if server.tlsCertificate != "" {
		server.httpServer.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return server
}

// withMiddleware wraps the router in the shared middleware. The list runs from the outermost layer
// inward: the request ID and trace context come first so every later layer, including the access log
// and panic recovery, can attach them, and the body limit sits closest to the handlers.
func (server *Server) withMiddleware(router http.Handler, corsAllowedMethods string) http.Handler {
	layers := []func(http.Handler) http.Handler{
		middleware.RequestID(),
		middleware.SecurityHeaders(server.tlsCertificate != ""),
		middleware.Metrics(server.metrics),
		middleware.RateLimit(server.rateLimiter, server.rateLimitKey),
		middleware.AccessLog(server.logger),
		middleware.CORS(server.allowedOrigins, corsAllowedMethods),
		middleware.PanicRecovery(server.logger),
		middleware.BodyLimit(server.maxBodyBytes),
	}
	handler := router
	for index := len(layers) - 1; index >= 0; index-- {
		handler = layers[index](handler)
	}
	return handler
}

// rateLimitKey assigns an API request to its limit class and keys it on the client address; requests
// outside /api/v1 have no limit.
func (server *Server) rateLimitKey(request *http.Request) (string, int) {
	if !strings.HasPrefix(request.URL.Path, "/api/v1/") {
		return "", 0
	}
	class, limit := "query", server.rateLimits.queries
	switch {
	case strings.HasSuffix(request.URL.Path, "/replays"):
		class, limit = "replay", server.rateLimits.replay
	case strings.HasPrefix(request.URL.Path, "/api/v1/auth/") || strings.HasSuffix(request.URL.Path, "/ingest-tokens"):
		class, limit = "auth", server.rateLimits.auth
	}
	return class + ":" + server.clientAddress(request), limit
}

// clientAddress is the address rate limits are keyed on; see config.ClientAddress for how forwarded
// headers are trusted.
func (server *Server) clientAddress(request *http.Request) string {
	return config.ClientAddress(request.RemoteAddr, request.Header.Values("X-Forwarded-For"), server.trustedProxies)
}

// ServeHTTP serves a request through the middleware and the router.
func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server.httpServer.Handler.ServeHTTP(w, r)
}

// Run listens and serves until applicationContext ends, then marks the server not ready, closes the
// stream hub so open streams return, and shuts down within the configured timeout.
func (server *Server) Run(applicationContext context.Context) error {
	listener, listenError := net.Listen("tcp", server.httpServer.Addr)
	if listenError != nil {
		return listenError
	}
	server.ready.Store(true)

	serveResult := make(chan error, 1)
	go func() {
		if server.tlsCertificate != "" {
			serveResult <- server.httpServer.ServeTLS(listener, server.tlsCertificate, server.tlsPrivateKey)
			return
		}
		serveResult <- server.httpServer.Serve(listener)
	}()

	select {
	case serveError := <-serveResult:
		server.ready.Store(false)
		if errors.Is(serveError, http.ErrServerClosed) {
			return nil
		}
		return serveError
	case <-applicationContext.Done():
		server.ready.Store(false)
		if server.streamHub != nil {
			server.streamHub.Close()
		}
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), server.shutdownTimeout)
		defer cancelShutdown()
		if shutdownError := server.httpServer.Shutdown(shutdownContext); shutdownError != nil {
			_ = server.httpServer.Close()
			return shutdownError
		}
		return nil
	}
}
