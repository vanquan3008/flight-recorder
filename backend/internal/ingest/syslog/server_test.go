package syslog

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestReceiveStoresNormalizedSecurityEvent(t *testing.T) {
	stored := make(chan telemetry.Event, 1)
	ingestion := config.IngestionConfig{ProjectID: "01900000-0000-7000-8000-000000000001", EnvironmentName: "development"}
	ingestionPipeline := pipeline.New(pipeline.Config{
		QueueSize: 1, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1,
		EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond,
	}, func(_ context.Context, events []telemetry.Event) error {
		stored <- events[0]
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
			t.Fatal(shutdownError)
		}
	}()

	server := New(config.SecurityConfig{MaxLineBytes: 1024, IdleTimeout: time.Second}, ingestion, ingestionPipeline, metrics.New("test", "test", "test"), slog.New(slog.NewTextHandler(io.Discard, nil)), "", "")
	reader, writer := net.Pipe()
	done := make(chan struct{})
	go func() { server.receive(context.Background(), reader); close(done) }()
	if _, writeError := writer.Write([]byte("CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8 dst=203.0.113.5 act=blocked\n")); writeError != nil {
		t.Fatal(writeError)
	}
	_ = writer.Close()

	select {
	case event := <-stored:
		if event.Security == nil || event.Security.Format != "cef" || event.Security.SourceIP != "10.0.0.8" {
			t.Fatalf("unexpected security event: %#v", event.Security)
		}
	case <-time.After(time.Second):
		t.Fatal("syslog record was not persisted")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("receiver did not stop after connection close")
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	server := New(config.SecurityConfig{SyslogAddress: "127.0.0.1:0", IdleTimeout: time.Second}, config.IngestionConfig{}, nil, metrics.New("test", "test", "test"), slog.New(slog.NewTextHandler(io.Discard, nil)), "", "")
	context, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(context) }()
	deadline := time.Now().Add(time.Second)
	for !server.Ready() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !server.Ready() {
		t.Fatal("server did not become ready")
	}
	cancel()
	select {
	case runError := <-done:
		if runError != nil {
			t.Fatal(runError)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

func TestTLSConfigurationRequiresTLS12AndCertificate(t *testing.T) {
	server := New(config.SecurityConfig{SyslogTLS: true, SyslogAddress: "127.0.0.1:0"}, config.IngestionConfig{}, nil, metrics.New("test", "test", "test"), slog.New(slog.NewTextHandler(io.Discard, nil)), "missing.crt", "missing.key")
	if _, listenError := server.listen(); listenError == nil {
		t.Fatal("expected missing certificate error")
	}
	if tlsConfig := server.tlsConfig(tls.Certificate{}); tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("minimum TLS version = %d", tlsConfig.MinVersion)
	}
}

func TestTLSConfigurationVerifiesClientCertificate(t *testing.T) {
	caCertificate, caKey := testCertificate(t, true, nil, nil, "tracepilot-test-ca")
	serverCertificate, serverKey := testCertificate(t, false, caCertificate, caKey, "localhost")
	clientCertificate, clientKey := testCertificate(t, false, caCertificate, caKey, "tracepilot-client")
	temporaryDirectory := t.TempDir()
	caPath := writeCertificate(t, temporaryDirectory, "ca.crt", caCertificate)
	serverCertificatePath := writeCertificate(t, temporaryDirectory, "server.crt", serverCertificate)
	serverKeyPath := writeKey(t, temporaryDirectory, "server.key", serverKey)
	clientCertificatePath := writeCertificate(t, temporaryDirectory, "client.crt", clientCertificate)
	clientKeyPath := writeKey(t, temporaryDirectory, "client.key", clientKey)

	server := New(config.SecurityConfig{
		SyslogTLS:          true,
		SyslogAddress:      "127.0.0.1:0",
		SyslogClientCAFile: caPath,
	}, config.IngestionConfig{}, nil, metrics.New("test", "test", "test"), slog.New(slog.NewTextHandler(io.Discard, nil)), serverCertificatePath, serverKeyPath)
	listener, listenError := server.listen()
	if listenError != nil {
		t.Fatal(listenError)
	}
	t.Cleanup(func() { _ = listener.Close() })

	acceptHandshake := func() error {
		connection, acceptError := listener.Accept()
		if acceptError != nil {
			return acceptError
		}
		defer connection.Close()
		return connection.(*tls.Conn).Handshake()
	}
	rootCAPool := x509.NewCertPool()
	if !rootCAPool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertificate.Raw})) {
		t.Fatal("failed to build test root CA pool")
	}
	withoutClientCertificate := make(chan error, 1)
	go func() { withoutClientCertificate <- acceptHandshake() }()
	clientConnection, _ := tls.Dial("tcp", listener.Addr().String(), &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootCAPool, ServerName: "localhost"})
	if clientConnection != nil {
		_ = clientConnection.Close()
	}
	if serverError := <-withoutClientCertificate; serverError == nil {
		t.Fatal("server did not reject TLS client without certificate")
	}

	clientPair, pairError := tls.LoadX509KeyPair(clientCertificatePath, clientKeyPath)
	if pairError != nil {
		t.Fatal(pairError)
	}
	withClientCertificate := make(chan error, 1)
	go func() { withClientCertificate <- acceptHandshake() }()
	clientConnection, clientError := tls.Dial("tcp", listener.Addr().String(), &tls.Config{
		MinVersion: tls.VersionTLS12, RootCAs: rootCAPool, ServerName: "localhost", Certificates: []tls.Certificate{clientPair},
	})
	if clientError != nil {
		t.Fatal(clientError)
	}
	_ = clientConnection.Close()
	if serverError := <-withClientCertificate; serverError != nil {
		t.Fatal(serverError)
	}
}

func testCertificate(t *testing.T, authority bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, commonName string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, keyError := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keyError != nil {
		t.Fatal(keyError)
	}
	serial, serialError := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if serialError != nil {
		t.Fatal(serialError)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: commonName}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), BasicConstraintsValid: true}
	if authority {
		template.IsCA = true
		template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature
	} else {
		template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		template.DNSNames = []string{commonName}
	}
	if parent == nil {
		parent = template
		parentKey = key
	}
	der, certificateError := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if certificateError != nil {
		t.Fatal(certificateError)
	}
	certificate, parseError := x509.ParseCertificate(der)
	if parseError != nil {
		t.Fatal(parseError)
	}
	return certificate, key
}

func writeCertificate(t *testing.T, directory, name string, certificate *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if writeError := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	return path
}

func writeKey(t *testing.T, directory, name string, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, marshalError := x509.MarshalPKCS8PrivateKey(key)
	if marshalError != nil {
		t.Fatal(marshalError)
	}
	path := filepath.Join(directory, name)
	if writeError := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	return path
}

func TestReceiveHandlesMultipleRecordsAndConnections(t *testing.T) {
	stored := make(chan telemetry.Event, 4)
	ingestion := config.IngestionConfig{ProjectID: "01900000-0000-7000-8000-000000000001", EnvironmentName: "development"}
	ingestionPipeline := pipeline.New(pipeline.Config{QueueSize: 4, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, events []telemetry.Event) error {
		for _, event := range events {
			stored <- event
		}
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	t.Cleanup(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
			t.Error(shutdownError)
		}
	})
	server := New(config.SecurityConfig{MaxLineBytes: 1024, IdleTimeout: time.Second}, ingestion, ingestionPipeline, metrics.New("test", "test", "test"), slog.New(slog.NewTextHandler(io.Discard, nil)), "", "")
	for connectionNumber := 0; connectionNumber < 2; connectionNumber++ {
		reader, writer := net.Pipe()
		done := make(chan struct{})
		go func() { server.receive(context.Background(), reader); close(done) }()
		payload := "<134>1 2026-08-31T10:00:00Z fw - - - CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8 act=blocked\n\n"
		if _, writeError := writer.Write([]byte(payload)); writeError != nil {
			t.Fatal(writeError)
		}
		_ = writer.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("receiver did not stop")
		}
	}
	for index := 0; index < 2; index++ {
		select {
		case event := <-stored:
			if event.Security == nil || event.Security.Format != "syslog+cef" {
				t.Fatalf("event %d = %#v", index, event.Security)
			}
		case <-time.After(time.Second):
			t.Fatal("record was not persisted")
		}
	}
}

func TestReceiveStopsOnIdleAndOversizedLine(t *testing.T) {
	ingestionPipeline := pipeline.New(pipeline.Config{QueueSize: 2, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error { return nil }, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	t.Cleanup(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
			t.Error(shutdownError)
		}
	})
	server := New(config.SecurityConfig{MaxLineBytes: 16, IdleTimeout: 20 * time.Millisecond}, config.IngestionConfig{ProjectID: "01900000-0000-7000-8000-000000000001", EnvironmentName: "development"}, ingestionPipeline, metrics.New("test", "test", "test"), slog.New(slog.NewTextHandler(io.Discard, nil)), "", "")
	reader, writer := net.Pipe()
	done := make(chan struct{})
	go func() { server.receive(context.Background(), reader); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle connection did not stop")
	}
	_ = writer.Close()

	reader, writer = net.Pipe()
	done = make(chan struct{})
	go func() { server.receive(context.Background(), reader); close(done) }()
	_, _ = writer.Write([]byte("01234567890123456789\n"))
	_ = writer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("oversized connection did not stop")
	}
}
