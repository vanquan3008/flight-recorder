package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func stopLogs(t *testing.T, exporter *logExporter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exporter.shutdown(ctx); err != nil {
		t.Error(err)
	}
}

func TestDemoExportsCorrelatedLogsAndRetriesSamePayload(t *testing.T) {
	payloads := make(chan []byte, 4)
	var calls atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/logs" || r.Header.Get("X-Environment") != "development" ||
			r.Header.Get("Authorization") != "Bearer test+token" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unexpected OTLP request method, path or headers")
		}
		payload, _ := io.ReadAll(r.Body)
		payloads <- payload
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"partialSuccess":{"rejectedLogRecords":"1"}}`)
	}))
	defer sink.Close()
	logger := slog.New(slog.DiscardHandler)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", sink.URL+"/v1/logs")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_HEADERS", "authorization=Bearer%20test+token,x-environment=development")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=wrong")
	exporter, err := configuredLogExporter(logger)
	if err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	app := demo{tracer: provider.Tracer(serviceName), logger: logger, logs: exporter,
		lookup: func(context.Context, int32) (refundPolicy, error) { return refundPolicy{version: 1, days: 14}, nil }}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"policy_version":1}`)))
	result := readAnswer(t, response)
	stopLogs(t, exporter)
	if response.Code != 200 || result.Answer != "You can request a refund within 14 days." {
		t.Fatalf("unexpected request response: %+v", result)
	}
	if calls.Load() != 2 || exporter.retries.Load() != 1 || exporter.rejected.Load() != 1 || exporter.accepted.Load() != 1 {
		t.Fatalf("requests=%d retries=%d rejected=%d accepted=%d", calls.Load(), exporter.retries.Load(),
			exporter.rejected.Load(), exporter.accepted.Load())
	}
	payload := <-payloads
	if !bytes.Equal(payload, <-payloads) || bytes.Contains(payload, []byte(result.Answer)) {
		t.Fatal("retry modified the payload or it contains raw model content")
	}
	var envelope struct {
		ResourceLogs []struct {
			Resource  struct{ Attributes []logAttribute }
			ScopeLogs []struct {
				Scope      struct{ Name string }
				LogRecords []logRecord
			}
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.ResourceLogs) != 1 || len(envelope.ResourceLogs[0].ScopeLogs) != 1 {
		t.Fatalf("unexpected resource/scope envelope: %+v", envelope)
	}
	group := envelope.ResourceLogs[0]
	scope := group.ScopeLogs[0]
	if group.Resource.Attributes[0].Value.String != serviceName || scope.Scope.Name != serviceName {
		t.Fatalf("wrong service or scope: %+v", group)
	}
	wants := map[string]string{"policy.retrieved": "policy.retrieve", "generation.completed": "chat " + modelName, "request.completed": "POST /ask"}
	spanIDs := make(map[string]string)
	for _, span := range recorder.Ended() {
		spanIDs[span.Name()] = span.SpanContext().SpanID().String()
	}
	for _, record := range scope.LogRecords {
		if name := wants[record.Body.String]; name == "" || record.TraceID != result.TraceID ||
			record.SpanID != spanIDs[name] || record.SeverityNumber != 9 {
			t.Fatalf("unexpected correlation: %+v", record)
		}
		if stamp, err := strconv.ParseInt(record.Time, 10, 64); err != nil || stamp <= 0 {
			t.Fatalf("invalid nanosecond timestamp: %q", record.Time)
		}
		delete(wants, record.Body.String)
	}
	if len(wants) != 0 {
		t.Fatalf("missing log events: %v", wants)
	}
}

func TestLogOutageDropsRatherThanBlockingRequests(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var received atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if received.Add(1) == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer sink.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	logger := slog.New(slog.DiscardHandler)
	exporter, err := newLogExporter(sink.URL, "", logger)
	if err != nil {
		t.Fatal(err)
	}
	defer stopLogs(t, exporter)
	record := logRecord{Body: logValue{String: "policy.retrieved"}}
	for range logBatchSize {
		exporter.enqueue(record)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no batch was sent")
	}
	for range logQueueSize {
		exporter.enqueue(record)
	}
	exporter.enqueue(logRecord{Body: logValue{String: strings.Repeat("s", logRecordBytes)}})
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	app := demo{tracer: provider.Tracer(serviceName), logger: logger, logs: exporter,
		lookup: func(context.Context, int32) (refundPolicy, error) { return refundPolicy{version: 2, days: 30}, nil }}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader("{}")))
	if response.Code != 200 || readAnswer(t, response).PolicyVersion != 2 || exporter.dropped.Load() != 4 {
		t.Fatalf("outage changed request or drop behavior: HTTP %d drops %d", response.Code, exporter.dropped.Load())
	}
	unblock()
	var concurrent sync.WaitGroup
	for range 4 {
		concurrent.Go(func() {
			for range 100 {
				exporter.enqueue(record)
			}
		})
	}
	stopLogs(t, exporter)
	concurrent.Wait()
	if exporter.retries.Load() != 0 || exporter.failed.Load() < logQueueSize+logBatchSize {
		t.Fatal("permanent HTTP errors were retried or failed records were unaccounted for")
	}
}

func TestLogFailureEventUsesNormalizedWarning(t *testing.T) {
	received := make(chan string, 1)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		received <- string(data)
		fmt.Fprint(w, "{}")
	}))
	defer sink.Close()
	logger := slog.New(slog.DiscardHandler)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "x-environment=development")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", sink.URL)
	exporter, err := configuredLogExporter(logger)
	if err != nil {
		t.Fatal(err)
	}
	if exporter.headers.Get("X-Environment") != "development" {
		t.Fatal("sender did not use shared OTLP headers")
	}
	provider := sdktrace.NewTracerProvider()
	app := demo{tracer: provider.Tracer(serviceName), logger: logger, logs: exporter,
		lookup: func(context.Context, int32) (refundPolicy, error) {
			return refundPolicy{}, errors.New("raw database error")
		}}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader("{}")))
	stopLogs(t, exporter)
	select {
	case payload := <-received:
		if !strings.Contains(payload, "policy.lookup_failed") || !strings.Contains(payload, "database_error") ||
			!strings.Contains(payload, `"severityNumber":13`) || strings.Contains(payload, "raw database error") {
			t.Fatalf("unexpected failure records: %s", payload)
		}
	default:
		t.Fatal("missing log export")
	}
}
