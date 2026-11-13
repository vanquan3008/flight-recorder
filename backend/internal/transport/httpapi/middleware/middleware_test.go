package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricMethod(t *testing.T) {
	if got := MetricMethod("GET"); got != "GET" {
		t.Errorf("got %q, want GET", got)
	}
	if got := MetricMethod("CUSTOM"); got != "OTHER" {
		t.Errorf("got %q, want OTHER", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options header")
	}
	if recorder.Header().Get("Strict-Transport-Security") == "" {
		t.Error("missing Strict-Transport-Security header when hsts is enabled")
	}
}

func TestRequestID(t *testing.T) {
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Header().Get(RequestIDHeader) == "" {
		t.Error("missing X-Request-ID header")
	}
	if recorder.Header().Get("traceparent") == "" {
		t.Error("missing traceparent header")
	}
}

func TestRequestID_PreservesIncomingTraceparent(t *testing.T) {
	incomingTrace := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", incomingTrace)
	handler.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("traceparent"); got != incomingTrace {
		t.Errorf("traceparent = %q, want %q", got, incomingTrace)
	}
}
