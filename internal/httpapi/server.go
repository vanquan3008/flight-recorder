package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/flight-observability/flight/internal/config"
	"github.com/flight-observability/flight/internal/metrics"
)

type Server struct {
	httpServer       *http.Server
	logger           *slog.Logger
	metrics          *metrics.Registry
	readinessCheck   func(context.Context) error
	readinessTimeout time.Duration
	maxBodyBytes     int64
	ready            atomic.Bool
	shutdownTimeout  time.Duration
}

func New(configuration config.Config, logger *slog.Logger, readinessCheck func(context.Context) error, metricRegistry *metrics.Registry) *Server {
	server := &Server{
		logger:           logger,
		metrics:          metricRegistry,
		readinessCheck:   readinessCheck,
		readinessTimeout: configuration.Database.ReadinessTimeout,
		maxBodyBytes:     configuration.HTTP.ManagementMaxBodyBytes,
		shutdownTimeout:  configuration.ShutdownTimeout,
	}
	router := http.NewServeMux()
	router.HandleFunc("GET /health/live", server.live)
	router.HandleFunc("GET /health/ready", server.readiness)
	router.Handle("GET /metrics", metricRegistry.Handler())

	var requestHandler http.Handler = router
	requestHandler = server.withBodyLimit(requestHandler)
	requestHandler = server.withPanicRecovery(requestHandler)
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
