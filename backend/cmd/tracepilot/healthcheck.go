package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultHealthcheckURL    = "http://127.0.0.1:8080/health/ready"
	maxHealthcheckBody       = 4096
	healthcheckCAFileEnv     = "TRACEPILOT_HEALTHCHECK_CA_FILE"
	healthcheckServerNameEnv = "TRACEPILOT_HEALTHCHECK_SERVER_NAME"
)

func healthcheck() int {
	healthcheckURL := resolveHealthcheckURL()
	healthcheckClient, clientError := newHealthcheckClient(healthcheckURL)
	if clientError != nil {
		slog.Error("healthcheck failed", "error", clientError)
		return 1
	}
	healthcheckResponse, requestError := healthcheckClient.Get(healthcheckURL)
	if requestError != nil {
		slog.Error("healthcheck failed", "error", requestError)
		return 1
	}
	defer healthcheckResponse.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(healthcheckResponse.Body, maxHealthcheckBody))
	if healthcheckResponse.StatusCode != http.StatusOK {
		slog.Error("healthcheck failed", "status", healthcheckResponse.StatusCode)
		return 1
	}
	return 0
}

func resolveHealthcheckURL() string {
	if healthcheckURL := os.Getenv("TRACEPILOT_HEALTHCHECK_URL"); healthcheckURL != "" {
		return healthcheckURL
	}
	certificateFile, configured := os.LookupEnv("TRACEPILOT_APP_TLS_CERTIFICATE_FILE")
	if !configured {
		certificateFile = os.Getenv("TRACEPILOT_TLS_CERTIFICATE_FILE")
	}
	if certificateFile != "" {
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
	if serverName := os.Getenv(healthcheckServerNameEnv); serverName != "" {
		tlsConfig.ServerName = serverName
	}
	if caFile := os.Getenv(healthcheckCAFileEnv); caFile != "" {
		certificatePEM, err := os.ReadFile(caFile)
		if err != nil {
			return http.Client{}, errors.New("read healthcheck CA file: " + err.Error())
		}
		rootCAs, err := x509.SystemCertPool()
		if err != nil || rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		if !rootCAs.AppendCertsFromPEM(certificatePEM) {
			return http.Client{}, errors.New("parse healthcheck CA file: invalid certificate PEM")
		}
		tlsConfig.RootCAs = rootCAs
	}
	healthcheckClient.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	return healthcheckClient, nil
}
