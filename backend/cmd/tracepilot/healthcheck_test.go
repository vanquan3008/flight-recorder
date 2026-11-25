package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthcheckAcceptsCertificateForHealthcheckSAN(t *testing.T) {
	certificate, certificatePEM := newHealthcheckCertificate(t)
	caFile := filepath.Join(t.TempDir(), "healthcheck-ca.pem")
	if err := os.WriteFile(caFile, certificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health/ready" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	defer server.Close()

	t.Setenv("TRACEPILOT_HEALTHCHECK_URL", server.URL+"/health/ready")
	t.Setenv(healthcheckCAFileEnv, caFile)
	if exitCode := healthcheck(); exitCode != 0 {
		t.Fatalf("healthcheck exit code = %d, want 0", exitCode)
	}
}

func TestResolveHealthcheckURLUsesExplicitHTTPSURL(t *testing.T) {
	t.Setenv("TRACEPILOT_APP_TLS_CERTIFICATE_FILE", "tls.crt")
	t.Setenv("TRACEPILOT_HEALTHCHECK_URL", "https://localhost:8080/health/ready")
	if got, want := resolveHealthcheckURL(), "https://localhost:8080/health/ready"; got != want {
		t.Fatalf("healthcheck URL = %q, want %q", got, want)
	}
}

func TestHealthcheckSupportsExplicitTLSServerName(t *testing.T) {
	certificate, certificatePEM := newHealthcheckCertificate(t)
	caFile := filepath.Join(t.TempDir(), "healthcheck-ca.pem")
	if err := os.WriteFile(caFile, certificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	defer server.Close()

	t.Setenv("TRACEPILOT_HEALTHCHECK_URL", "https://127.0.0.1:8080/health/ready")
	t.Setenv(healthcheckCAFileEnv, caFile)
	t.Setenv(healthcheckServerNameEnv, "localhost")
	client, clientError := newHealthcheckClient("https://127.0.0.1:8080/health/ready")
	if clientError != nil {
		t.Fatal(clientError)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.ServerName != "localhost" {
		t.Fatalf("TLS server name = %#v", transport.TLSClientConfig)
	}
}

func newHealthcheckCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "tracepilot-healthcheck"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
	certificate, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, certificatePEM
}

func TestResolveHealthcheckURLFollowsListenerTLS(t *testing.T) {
	t.Setenv("TRACEPILOT_HEALTHCHECK_URL", "")
	if err := os.Unsetenv("TRACEPILOT_HEALTHCHECK_URL"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACEPILOT_APP_TLS_CERTIFICATE_FILE", "tls.crt")
	if got, want := resolveHealthcheckURL(), "https://127.0.0.1:8080/health/ready"; got != want {
		t.Fatalf("healthcheck URL = %q, want %q", got, want)
	}
}
