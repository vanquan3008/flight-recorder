// Package syslog receives newline-delimited Syslog over TCP or TLS and feeds
// it through the same durable pipeline as OTLP.
package syslog

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	securityingest "github.com/vanquan3008/tracepilot/internal/security"
)

type Server struct {
	configuration config.SecurityConfig
	ingestion     config.IngestionConfig
	pipeline      *pipeline.Pipeline
	metrics       *metrics.Registry
	logger        *slog.Logger
	certificate   string
	privateKey    string
	ready         atomic.Bool
	connections   sync.Map
	receivers     sync.WaitGroup
	active        atomic.Int64
	// connectionSlots bounds the number of concurrent syslog connections so a
	// TCP connect flood cannot exhaust the server's goroutine pool.
	connectionSlots *semaphore.Weighted
}

func New(configuration config.SecurityConfig, ingestion config.IngestionConfig, pipeline *pipeline.Pipeline, metricRegistry *metrics.Registry, logger *slog.Logger, certificate, privateKey string) *Server {
	return &Server{
		configuration:   configuration,
		ingestion:       ingestion,
		pipeline:        pipeline,
		metrics:         metricRegistry,
		logger:          logger,
		certificate:     certificate,
		privateKey:      privateKey,
		connectionSlots: semaphore.NewWeighted(int64(configuration.MaxConnections)),
	}
}

func (server *Server) Ready() bool { return server.ready.Load() }

func (server *Server) Run(ctx context.Context) error {
	listener, listenError := server.listen()
	if listenError != nil {
		return listenError
	}
	defer listener.Close()
	defer server.receivers.Wait()
	defer server.closeConnections()
	server.ready.Store(true)
	defer server.ready.Store(false)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
		server.closeConnections()
	}()

	for {
		connection, acceptError := listener.Accept()
		if acceptError != nil {
			if ctx.Err() != nil || errors.Is(acceptError, net.ErrClosed) {
				return nil
			}
			if temporary, ok := acceptError.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				server.logger.Warn("temporary syslog accept failure", "error", acceptError)
				continue
			}
			return acceptError
		}

		// Try to acquire a slot without blocking. If at capacity, reject the
		// connection immediately rather than queuing it — this keeps memory use
		// bounded and gives operators a clear signal (connection refused) that
		// the limit needs raising.
		if !server.connectionSlots.TryAcquire(1) {
			server.logger.Warn("syslog connection limit reached, rejecting connection",
				"peer", connection.RemoteAddr().String(),
				"limit", server.configuration.MaxConnections,
			)
			_ = connection.Close()
			continue
		}

		server.connections.Store(connection, struct{}{})
		server.metrics.SetSyslogConnections(int(server.active.Add(1)))
		server.receivers.Add(1)
		go func() {
			defer server.receivers.Done()
			defer server.connectionSlots.Release(1)
			server.receive(ctx, connection)
		}()
	}
}

func (server *Server) closeConnections() {
	server.connections.Range(func(key, _ any) bool {
		_ = key.(net.Conn).Close()
		return true
	})
}

func (server *Server) listen() (net.Listener, error) {
	if !server.configuration.SyslogTLS {
		return net.Listen("tcp", server.configuration.SyslogAddress)
	}
	pair, pairError := tls.LoadX509KeyPair(server.certificate, server.privateKey)
	if pairError != nil {
		return nil, fmt.Errorf("load syslog TLS certificate: %w", pairError)
	}
	tlsConfig := server.tlsConfig(pair)
	if clientCAFile := strings.TrimSpace(server.configuration.SyslogClientCAFile); clientCAFile != "" {
		caPEM, readError := os.ReadFile(clientCAFile)
		if readError != nil {
			return nil, fmt.Errorf("read syslog client CA: %w", readError)
		}
		clientCAPool := x509.NewCertPool()
		if !clientCAPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse syslog client CA: %s contains no certificates", clientCAFile)
		}
		tlsConfig.ClientCAs = clientCAPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tls.Listen("tcp", server.configuration.SyslogAddress, tlsConfig)
}

func (server *Server) tlsConfig(certificate tls.Certificate) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
}

func (server *Server) receive(ctx context.Context, connection net.Conn) {
	defer server.connections.Delete(connection)
	defer server.metrics.SetSyslogConnections(int(server.active.Add(-1)))
	defer connection.Close()

	if server.pipeline == nil {
		server.logger.Warn("syslog receiver has no ingestion pipeline")
		return
	}

	scanner := bufio.NewScanner(connection)
	initialBufferSize := min(server.configuration.MaxLineBytes, 4096)
	if initialBufferSize < 1 {
		initialBufferSize = 1
	}
	scanner.Buffer(make([]byte, initialBufferSize), server.configuration.MaxLineBytes)

	for {
		_ = connection.SetReadDeadline(time.Now().Add(server.configuration.IdleTimeout))
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		event := securityingest.FromSyslogWithSourceIdentity(
			server.ingestion.ProjectID,
			server.ingestion.EnvironmentName,
			connection.RemoteAddr().String(),
			syslogClientIdentity(connection),
			line,
			time.Now().UTC(),
		)
		if _, submitError := server.pipeline.Submit(ctx, []telemetry.Event{event}); submitError != nil {
			server.metrics.ObserveSyslogRecord("rejected", securityFormat(event))
			if ctx.Err() == nil {
				server.logger.Warn("discarded syslog event",
					"peer", connection.RemoteAddr().String(),
					"error", submitError,
				)
			}
			return
		}
		server.metrics.ObserveSyslogRecord("accepted", securityFormat(event))
	}

	if scanError := scanner.Err(); scanError != nil && ctx.Err() == nil {
		server.logger.Warn("syslog connection ended with read error",
			"peer", connection.RemoteAddr().String(),
			"error", scanError,
		)
	}
}

func syslogClientIdentity(connection net.Conn) string {
	tlsConnection, ok := connection.(*tls.Conn)
	if !ok {
		return ""
	}
	state := tlsConnection.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ""
	}
	certificate := state.PeerCertificates[0]
	if len(certificate.URIs) > 0 {
		return certificate.URIs[0].String()
	}
	if len(certificate.DNSNames) > 0 {
		return certificate.DNSNames[0]
	}
	return certificate.Subject.CommonName
}

func securityFormat(event telemetry.Event) string {
	if event.Security == nil {
		return "unknown"
	}
	return event.Security.Format
}
