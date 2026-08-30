package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultHealthcheckURL = "http://127.0.0.1:8080/health/ready"
	maxHealthcheckBody    = 4096
	healthcheckCAFileEnv  = "TRACEPILOT_HEALTHCHECK_CA_FILE"
)

func healthcheck() int {
	healthcheckURL := resolveHealthcheckURL()
	healthcheckClient, clientError := newHealthcheckClient(healthcheckURL)
	if clientError != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", clientError)
		return 1
	}
	healthcheckResponse, requestError := healthcheckClient.Get(healthcheckURL)
	if requestError != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", requestError)
		return 1
	}
	defer healthcheckResponse.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(healthcheckResponse.Body, maxHealthcheckBody))
	if healthcheckResponse.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", healthcheckResponse.StatusCode)
		return 1
	}
	return 0
}

func resolveHealthcheckURL() string {
	if healthcheckURL := os.Getenv("TRACEPILOT_HEALTHCHECK_URL"); healthcheckURL != "" {
		return healthcheckURL
	}
	if os.Getenv("TRACEPILOT_TLS_CERTIFICATE_FILE") != "" {
		return "https://127.0.0.1:8080/health/ready"
	}
	return defaultHealthcheckURL
}

func newHealthcheckClient(healthcheckURL string) (http.Client, error) {
	healthcheckClient := http.Client{Timeout: 2 * time.Second}
	if !strings.HasPrefix(healthcheckURL, "https://") {
		return healthcheckClient, nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile := os.Getenv(healthcheckCAFileEnv); caFile != "" {
		certificatePEM, err := os.ReadFile(caFile)
		if err != nil {
			return http.Client{}, fmt.Errorf("read healthcheck CA file: %w", err)
		}
		rootCAs, err := x509.SystemCertPool()
		if err != nil || rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		if !rootCAs.AppendCertsFromPEM(certificatePEM) {
			return http.Client{}, fmt.Errorf("parse healthcheck CA file %q", caFile)
		}
		tlsConfig.RootCAs = rootCAs
	}
	healthcheckClient.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	return healthcheckClient, nil
}
