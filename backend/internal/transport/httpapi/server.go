package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"time"

	incidentapp "github.com/vanquan3008/tracepilot/internal/application/incident"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/webui"
)

type Server struct {
	httpServer              *http.Server
	logger                  *slog.Logger
	metrics                 *metrics.Registry
	metricsHandler          http.Handler
	readinessCheck          func(context.Context) error
	readinessTimeout        time.Duration
	environmentID           string
	maxBodyBytes            int64
	ready                   atomic.Bool
	shutdownTimeout         time.Duration
	backend                 Backend
	incidentCommands        *incidentapp.Service
	replay                  ReplayRunner
	replayMaxRequestBytes   int64
	allowedOrigins          map[string]struct{}
	trustedProxies          []netip.Prefix
	queryGate               chan struct{}
	requestTimeout          time.Duration
	corsAllowedMethods      string
	metricsRequireAuth      bool
	rateLimiter             *requestRateLimiter
	rateLimitRequests       int
	rateLimitAuthRequests   int
	rateLimitReplayRequests int
	certificateFile         string
	privateKeyFile          string
	webhooksEnabled         bool
}

func New(configuration config.Config, logger *slog.Logger, readinessCheck func(context.Context) error, metricRegistry *metrics.Registry, backend Backend, replayRunner ReplayRunner) *Server {
	server := &Server{
		logger:                  logger,
		metrics:                 metricRegistry,
		metricsHandler:          metricRegistry.Handler(),
		readinessCheck:          readinessCheck,
		readinessTimeout:        configuration.Database.ReadinessTimeout,
		environmentID:           configuration.Ingestion.EnvironmentID,
		maxBodyBytes:            configuration.HTTP.ManagementMaxBodyBytes,
		shutdownTimeout:         configuration.ShutdownTimeout,
		backend:                 backend,
		incidentCommands:        incidentapp.New(backend),
		replay:                  replayRunner,
		replayMaxRequestBytes:   configuration.Replay.MaxRequestBytes,
		allowedOrigins:          make(map[string]struct{}, len(configuration.HTTP.AllowedOrigins)),
		corsAllowedMethods:      corsMethods(apiRouteDefinitions),
		metricsRequireAuth:      configuration.Production,
		rateLimiter:             newRequestRateLimiter(configuration.HTTP.RateLimitWindow),
		rateLimitRequests:       configuration.HTTP.RateLimitRequests,
		rateLimitAuthRequests:   configuration.HTTP.RateLimitAuthRequests,
		rateLimitReplayRequests: configuration.HTTP.RateLimitReplayRequests,
		certificateFile:         configuration.TLS.CertificateFile,
		privateKeyFile:          configuration.TLS.PrivateKeyFile,
		webhooksEnabled:         configuration.Notifications.Enabled,
	}
	if server.rateLimitRequests <= 0 {
		server.rateLimitRequests = 120
	}
	if server.rateLimitAuthRequests <= 0 {
		server.rateLimitAuthRequests = 20
	}
	if server.rateLimitReplayRequests <= 0 {
		server.rateLimitReplayRequests = 10
	}
	for _, origin := range configuration.HTTP.AllowedOrigins {
		server.allowedOrigins[origin] = struct{}{}
	}
	server.trustedProxies = configuration.HTTP.TrustedProxyPrefixes()
	server.queryGate = make(chan struct{}, positiveOr(configuration.HTTP.MaxConcurrentQueries, defaultConcurrentQueries))
	server.requestTimeout = configuration.HTTP.RequestTimeout
	if server.requestTimeout <= 0 {
		server.requestTimeout = defaultRequestTimeout
	}
	router := http.NewServeMux()
	router.Handle("GET /", webui.New())
	router.HandleFunc("GET /openapi.json", server.openAPI)
	router.HandleFunc("GET /health/live", server.live)
	router.HandleFunc("GET /health/ready", server.readiness)
	router.HandleFunc("GET /metrics", server.metricsEndpoint)
	// Keep the query routes available whenever the database is available. When
	// authentication has not been configured they return an explicit 401 instead
	// of falling through to the dashboard and looking like a missing route.
	if backend != nil {
		server.registerAPIRoutes(router)
	}

	var requestHandler http.Handler = router
	requestHandler = server.withBodyLimit(requestHandler)
	requestHandler = server.withPanicRecovery(requestHandler)
	requestHandler = server.withCORS(requestHandler)
	requestHandler = server.withAccessLog(requestHandler)
	requestHandler = server.withRateLimit(requestHandler)
	requestHandler = server.withMetrics(requestHandler)
	requestHandler = server.withSecurityHeaders(requestHandler)
	requestHandler = server.withRequestID(requestHandler)
	server.httpServer = &http.Server{
		Addr:              configuration.HTTP.Address,
		Handler:           requestHandler,
		ReadHeaderTimeout: configuration.HTTP.ReadHeaderTimeout,
		ReadTimeout:       configuration.HTTP.ReadTimeout,
		WriteTimeout:      configuration.HTTP.WriteTimeout,
		IdleTimeout:       configuration.HTTP.IdleTimeout,
		MaxHeaderBytes:    configuration.HTTP.MaxHeaderBytes,
	}
	if server.certificateFile != "" {
		server.httpServer.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return server
}

func (server *Server) Run(applicationContext context.Context) error {
	listener, listenError := net.Listen("tcp", server.httpServer.Addr)
	if listenError != nil {
		return listenError
	}
	server.ready.Store(true)

	serveResult := make(chan error, 1)
	go func() {
		if server.certificateFile != "" {
			serveResult <- server.httpServer.ServeTLS(listener, server.certificateFile, server.privateKeyFile)
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
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), server.shutdownTimeout)
		defer cancelShutdown()
		if shutdownError := server.httpServer.Shutdown(shutdownContext); shutdownError != nil {
			_ = server.httpServer.Close()
			return shutdownError
		}
		return nil
	}
}

func (server *Server) metricsEndpoint(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.metricsRequireAuth {
		server.metricsHandler.ServeHTTP(responseWriter, request)
		return
	}
	if server.backend == nil {
		writeAPIError(responseWriter, http.StatusServiceUnavailable, "UNAVAILABLE", "metrics authentication is unavailable")
		return
	}
	if _, ok := server.authenticateScope(responseWriter, request, "read"); !ok {
		return
	}
	server.metricsHandler.ServeHTTP(responseWriter, request)
}

func (server *Server) live(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, `{"status":"ok"}`)
}

func (server *Server) readiness(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.ready.Load() {
		server.metrics.SetDatabaseReady(false)
		writeJSON(responseWriter, http.StatusServiceUnavailable, `{"status":"not_ready"}`)
		return
	}
	readinessContext, cancelReadiness := context.WithTimeout(request.Context(), server.readinessTimeout)
	defer cancelReadiness()
	if readinessError := server.readinessCheck(readinessContext); readinessError != nil {
		server.metrics.SetDatabaseReady(false)
		writeJSON(responseWriter, http.StatusServiceUnavailable, `{"status":"not_ready"}`)
		return
	}
	server.metrics.SetDatabaseReady(true)
	writeJSON(responseWriter, http.StatusOK, `{"status":"ready"}`)
}

func writeJSON(responseWriter http.ResponseWriter, statusCode int, responseBody string) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.WriteHeader(statusCode)
	_, _ = responseWriter.Write([]byte(responseBody + "\n"))
}
