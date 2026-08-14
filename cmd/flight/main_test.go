package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	t.Setenv("FLIGHT_HEALTHCHECK_URL", healthServer.URL)
	if exitCode := healthcheck(); exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
}

func TestHealthcheckRejectsUnreadyServer(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer healthServer.Close()
	t.Setenv("FLIGHT_HEALTHCHECK_URL", healthServer.URL)
	if exitCode := healthcheck(); exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}
