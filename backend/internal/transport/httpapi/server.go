package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/replay"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

type Server struct {
	httpServer            *http.Server
	logger                *slog.Logger
	metrics               *metrics.Registry
	metricsHandler        http.Handler
	readinessCheck        func(context.Context) error
	readinessTimeout      time.Duration
	environmentID         string
	maxBodyBytes          int64
	ready                 atomic.Bool
	shutdownTimeout       time.Duration
	database              *postgres.Database
	replay                *replay.Executor
	replayMaxRequestBytes int64
	allowedOrigins        map[string]struct{}
	metricsRequireAuth    bool
	certificateFile       string
	privateKeyFile        string
}

func New(configuration config.Config, logger *slog.Logger, readinessCheck func(context.Context) error, metricRegistry *metrics.Registry, database *postgres.Database, replayExecutor *replay.Executor) *Server {
	server := &Server{
		logger:                logger,
		metrics:               metricRegistry,
		metricsHandler:        metricRegistry.Handler(),
		readinessCheck:        readinessCheck,
		readinessTimeout:      configuration.Database.ReadinessTimeout,
		environmentID:         configuration.Ingestion.EnvironmentID,
		maxBodyBytes:          configuration.HTTP.ManagementMaxBodyBytes,
		shutdownTimeout:       configuration.ShutdownTimeout,
		database:              database,
		replay:                replayExecutor,
		replayMaxRequestBytes: configuration.Replay.MaxRequestBytes,
		allowedOrigins:        make(map[string]struct{}, len(configuration.HTTP.AllowedOrigins)),
		metricsRequireAuth:    configuration.Production,
		certificateFile:       configuration.TLS.CertificateFile,
		privateKeyFile:        configuration.TLS.PrivateKeyFile,
	}
	for _, origin := range configuration.HTTP.AllowedOrigins {
		server.allowedOrigins[origin] = struct{}{}
	}
	router := http.NewServeMux()
	router.HandleFunc("GET /openapi.json", server.openAPI)
	router.HandleFunc("GET /health/live", server.live)
	router.HandleFunc("GET /health/ready", server.readiness)
	router.HandleFunc("GET /metrics", server.metricsEndpoint)
	// Keep the query routes available whenever the database is available. When
	// authentication has not been configured they return an explicit 401 instead
	// of falling through to the dashboard and looking like a missing route.
	if database != nil {
		router.HandleFunc("GET /api/v1/traces", server.listTraces)
		router.HandleFunc("GET /api/v1/traces/{traceID}", server.getTrace)
		router.HandleFunc("GET /api/v1/traces/{traceID}/logs", server.listTraceLogs)
		router.HandleFunc("GET /api/v1/logs", server.listLogs)
		router.HandleFunc("GET /api/v1/logs/{eventID}", server.getLog)
		router.HandleFunc("GET /api/v1/services", server.listServices)
		router.HandleFunc("GET /api/v1/services/{service}/dependencies", server.listServiceDependencies)
		router.HandleFunc("GET /api/v1/incidents", server.listIncidents)
		router.HandleFunc("GET /api/v1/incidents/{incidentID}", server.getIncident)
		router.HandleFunc("GET /api/v1/incidents/{incidentID}/occurrences", server.listIncidentOccurrences)
		router.HandleFunc("GET /api/v1/incidents/{incidentID}/events", server.listIncidentEvents)
		router.HandleFunc("GET /api/v1/incidents/{incidentID}/status-changes", server.listIncidentStatusChanges)
		router.HandleFunc("GET /api/v1/incidents/{incidentID}/executions", server.listAllowedRequestExecutions)
		router.HandleFunc("GET /api/v1/incidents/{incidentID}/executions/{executionID}", server.getAllowedRequestExecution)
		router.HandleFunc("PATCH /api/v1/incidents/{incidentID}", server.updateIncident)
		router.HandleFunc("GET /api/v1/api-keys", server.listAPIKeys)
		router.HandleFunc("GET /api/v1/api-key-events", server.listAPIKeyEvents)
		router.HandleFunc("POST /api/v1/api-keys", server.createAPIKey)
		router.HandleFunc("POST /api/v1/api-keys/{keyID}/rotate", server.rotateAPIKey)
		router.HandleFunc("DELETE /api/v1/api-keys/{keyID}", server.revokeAPIKey)
		if replayExecutor != nil {
			router.HandleFunc("POST /api/v1/incidents/{incidentID}/replays", server.replayIncident)
		}
	}

	var requestHandler http.Handler = router
	requestHandler = server.withBodyLimit(requestHandler)
	requestHandler = server.withPanicRecovery(requestHandler)
	requestHandler = server.withCORS(requestHandler)
	requestHandler = server.withAccessLog(requestHandler)
	requestHandler = server.withMetrics(requestHandler)
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
	if server.database == nil {
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
