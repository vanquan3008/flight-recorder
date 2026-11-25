package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func decodeLogLine(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.Unmarshal(output.Bytes(), &line); err != nil {
		t.Fatalf("decode log line %q: %v", output.String(), err)
	}
	return line
}

func assertField(t *testing.T, line map[string]any, key string, want any) {
	t.Helper()
	if got := line[key]; got != want {
		t.Errorf("%s = %v, want %v", key, got, want)
	}
}

func assertNoField(t *testing.T, line map[string]any, key string) {
	t.Helper()
	if value, present := line[key]; present {
		t.Errorf("%s = %v, want it absent", key, value)
	}
}

func TestLoggerAddsTraceAndServiceFields(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger(&output, &Options{ServiceName: "tracepilot-test", ServiceVersion: "v1.0.0-test"})
	ctx := WithTraceContext(context.Background(), TraceContext{TraceID: sampleTraceID, SpanID: sampleSpanID, RequestID: "req-12345"})

	logger.InfoContext(ctx, "processing order", "amount", 100)

	line := decodeLogLine(t, &output)
	assertField(t, line, "msg", "processing order")
	assertField(t, line, "amount", float64(100))
	assertField(t, line, traceIDKey, sampleTraceID)
	assertField(t, line, spanIDKey, sampleSpanID)
	assertField(t, line, requestIDKey, "req-12345")
	assertField(t, line, serviceNameKey, "tracepilot-test")
	assertField(t, line, serviceVersionKey, "v1.0.0-test")
}

func TestLoggerWithoutTraceContextAddsOnlyServiceFields(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger(&output, &Options{ServiceName: "tracepilot-test"})

	logger.Info("plain log without context", "key", "val")

	line := decodeLogLine(t, &output)
	assertField(t, line, "msg", "plain log without context")
	assertField(t, line, serviceNameKey, "tracepilot-test")
	assertNoField(t, line, traceIDKey)
	assertNoField(t, line, spanIDKey)
	assertNoField(t, line, requestIDKey)
	assertNoField(t, line, serviceVersionKey)
}

func TestLoggerKeepsFieldsTheCallerAlreadySet(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger(&output, &Options{ServiceName: "tracepilot-test"})
	ctx := WithTraceContext(context.Background(), TraceContext{TraceID: sampleTraceID, RequestID: "from-context"})

	logger.InfoContext(ctx, "explicit", requestIDKey, "from-caller", serviceNameKey, "worker")

	line := decodeLogLine(t, &output)
	assertField(t, line, requestIDKey, "from-caller")
	assertField(t, line, serviceNameKey, "worker")
	assertField(t, line, traceIDKey, sampleTraceID)
}

func TestLoggerDefaultsToInfoLevel(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger(&output, nil)

	logger.Debug("hidden")
	if output.Len() != 0 {
		t.Fatalf("debug record was written at the default level: %s", output.String())
	}

	logger.Info("visible")
	assertField(t, decodeLogLine(t, &output), "msg", "visible")
}

func TestLoggerHonoursConfiguredLevel(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger(&output, &Options{Level: slog.LevelWarn})

	logger.Info("hidden")
	if output.Len() != 0 {
		t.Fatalf("info record was written at warn level: %s", output.String())
	}
}
