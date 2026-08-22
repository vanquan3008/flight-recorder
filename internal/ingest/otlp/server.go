package otlp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/flight-observability/flight/internal/config"
	"google.golang.org/grpc"
)

type Server struct {
	grpcAddress     string
	httpAddress     string
	grpcServer      *grpc.Server
	httpServer      *http.Server
	receiver        *Receiver
	maxRequestBytes int64
	shutdownTimeout time.Duration
	certificateFile string
	privateKeyFile  string
	ready           atomic.Bool
}

func NewServer(configuration config.Config, receiver *Receiver) *Server {
	server := &Server{
		grpcAddress:     configuration.Ingestion.GRPCAddress,
		httpAddress:     configuration.Ingestion.HTTPAddress,
		grpcServer:      newGRPCServer(receiver, int(configuration.Ingestion.MaxRequestBytes), nil),
		receiver:        receiver,
		maxRequestBytes: configuration.Ingestion.MaxRequestBytes,
		shutdownTimeout: configuration.ShutdownTimeout,
		certificateFile: configuration.TLS.CertificateFile,
		privateKeyFile:  configuration.TLS.PrivateKeyFile,
	}
	server.httpServer = newHTTPServer(configuration, server)
	return server
}

func (server *Server) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	if server.tlsEnabled() {
		tlsConfig, tlsError := server.loadTLSConfig()
		if tlsError != nil {
			return tlsError
		}
		server.grpcServer = newGRPCServer(server.receiver, int(server.maxRequestBytes), tlsConfig)
	}
	grpcListener, grpcListenError := net.Listen("tcp", server.grpcAddress)
	if grpcListenError != nil {
		return grpcListenError
	}
	httpListener, httpListenError := net.Listen("tcp", server.httpAddress)
	if httpListenError != nil {
		_ = grpcListener.Close()
		return httpListenError
	}
	server.ready.Store(true)
	defer server.ready.Store(false)

	results := make(chan error, 2)
	go func() { results <- server.grpcServer.Serve(grpcListener) }()
	go func() {
		if server.tlsEnabled() {
			results <- server.httpServer.ServeTLS(httpListener, server.certificateFile, server.privateKeyFile)
			return
		}
		results <- server.httpServer.Serve(httpListener)
	}()

	var runError error
	consumed := 0
	select {
	case runError = <-results:
		consumed = 1
	case <-ctx.Done():
	}
	server.ready.Store(false)
	shutdownError := server.shutdown()
	for consumed < 2 {
		serveError := <-results
		if !errors.Is(serveError, grpc.ErrServerStopped) && !errors.Is(serveError, http.ErrServerClosed) {
			runError = errors.Join(runError, serveError)
		}
		consumed++
	}
	return errors.Join(runError, shutdownError)
}

func (server *Server) tlsEnabled() bool {
	return server.certificateFile != ""
}

func (server *Server) loadTLSConfig() (*tls.Config, error) {
	certificate, certificateError := tls.LoadX509KeyPair(server.certificateFile, server.privateKeyFile)
	if certificateError != nil {
		return nil, certificateError
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}, nil
}

func (server *Server) Ready() bool {
	return server.ready.Load()
}

func (server *Server) shutdown() error {
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), server.shutdownTimeout)
	defer cancelShutdown()
	httpResult := make(chan error, 1)
	go func() { httpResult <- server.httpServer.Shutdown(shutdownContext) }()
	grpcDone := make(chan struct{})
	go func() {
		server.grpcServer.GracefulStop()
		close(grpcDone)
	}()

	var shutdownError error
	select {
	case shutdownError = <-httpResult:
	case <-shutdownContext.Done():
		shutdownError = shutdownContext.Err()
		_ = server.httpServer.Close()
	}
	select {
	case <-grpcDone:
	case <-shutdownContext.Done():
		server.grpcServer.Stop()
		shutdownError = errors.Join(shutdownError, shutdownContext.Err())
	}
	return shutdownError
}
