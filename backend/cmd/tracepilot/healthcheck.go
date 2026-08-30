package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultHealthcheckURL = "http://127.0.0.1:8080/health/ready"
	maxHealthcheckBody    = 4096
)

func healthcheck() int {
	healthcheckURL := os.Getenv("TRACEPILOT_HEALTHCHECK_URL")
	if healthcheckURL == "" {
		healthcheckURL = defaultHealthcheckURL
	}

	healthcheckClient := http.Client{Timeout: 2 * time.Second}
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
