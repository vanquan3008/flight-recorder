package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRequestIDReplacesUnsafeIncomingValues(t *testing.T) {
	var seenRequestID, seenTraceparent string
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRequestID, seenTraceparent = r.Header.Get(RequestIDHeader), r.Header.Get(traceparentHeader)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "bad id\nwith newline")
	request.Header.Set(traceparentHeader, "00-00000000000000000000000000000000-00f067aa0ba902b7-01")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	requestID := recorder.Header().Get(RequestIDHeader)
	if !safeRequestID.MatchString(requestID) || requestID != seenRequestID {
		t.Fatalf("request ID = %q (handler saw %q), want one safe generated value", requestID, seenRequestID)
	}
	traceparent := recorder.Header().Get(traceparentHeader)
	if traceparent == "" || traceparent != seenTraceparent {
		t.Fatalf("traceparent = %q (handler saw %q), want one value", traceparent, seenTraceparent)
	}
	if strings.Contains(traceparent, "00000000000000000000000000000000") {
		t.Fatalf("traceparent = %q kept the all-zero incoming trace ID", traceparent)
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
