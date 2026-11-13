package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseTraceparent(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantOK    bool
		wantTrace string
		wantSpan  string
		wantFlags string
	}{
		{
			name:      "valid standard header",
			header:    "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantOK:    true,
			wantTrace: "4bf92f3577b34da6a3ce929d0e0e4736",
			wantSpan:  "00f067aa0ba902b7",
			wantFlags: "01",
		},
		{
			name:      "valid with whitespace and uppercase",
			header:    "  00-4BF92F3577B34DA6A3CE929D0E0E4736-00F067AA0BA902B7-00  ",
			wantOK:    true,
			wantTrace: "4bf92f3577b34da6a3ce929d0e0e4736",
			wantSpan:  "00f067aa0ba902b7",
			wantFlags: "00",
		},
		{
			name:   "invalid length",
			header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
			wantOK: false,
		},
		{
			name:   "invalid version ff",
			header: "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantOK: false,
		},
		{
			name:   "all zero trace_id is invalid",
			header: "00-00000000000000000000000000000000-00f067aa0ba902b7-01",
			wantOK: false,
		},
		{
			name:   "all zero span_id is invalid",
			header: "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
			wantOK: false,
		},
		{
			name:   "non-hex characters",
			header: "00-4bf92f3577b34da6a3ce929d0e0e473g-00f067aa0ba902b7-01",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, ok := ParseTraceparent(tt.header)
			if ok != tt.wantOK {
				t.Fatalf("ParseTraceparent(%q) ok = %v, want %v", tt.header, ok, tt.wantOK)
			}
			if ok {
				if tc.TraceID != tt.wantTrace {
					t.Errorf("TraceID = %q, want %q", tc.TraceID, tt.wantTrace)
				}
				if tc.SpanID != tt.wantSpan {
					t.Errorf("SpanID = %q, want %q", tc.SpanID, tt.wantSpan)
				}
				if tc.TraceFlags != tt.wantFlags {
					t.Errorf("TraceFlags = %q, want %q", tc.TraceFlags, tt.wantFlags)
				}
			}
		})
	}
}

func TestFormatTraceparent(t *testing.T) {
	tc := TraceContext{
		TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:     "00f067aa0ba902b7",
		TraceFlags: "01",
	}
	want := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if got := FormatTraceparent(tc); got != want {
		t.Fatalf("FormatTraceparent() = %q, want %q", got, want)
	}
}

func TestNewIDs(t *testing.T) {
	traceID := NewTraceID()
	if len(traceID) != 32 {
		t.Errorf("NewTraceID() length = %d, want 32", len(traceID))
	}
	spanID := NewSpanID()
	if len(spanID) != 16 {
		t.Errorf("NewSpanID() length = %d, want 16", len(spanID))
	}
}

func TestContextHandler_InjectsAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, &Options{
		Level:          slog.LevelInfo,
		ServiceName:    "tracepilot-test",
		ServiceVersion: "v1.0.0-test",
	})

	ctx := context.Background()
	ctx = WithTraceContext(ctx, TraceContext{
		TraceID:   "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:    "00f067aa0ba902b7",
		RequestID: "req-12345",
	})

	logger.InfoContext(ctx, "processing order", "amount", 100)

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode log json: %v, body: %s", err, buf.String())
	}

	if payload["msg"] != "processing order" {
		t.Errorf("expected msg 'processing order', got %v", payload["msg"])
	}
	if payload["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace_id, got %v", payload["trace_id"])
	}
	if payload["span_id"] != "00f067aa0ba902b7" {
		t.Errorf("expected span_id, got %v", payload["span_id"])
	}
	if payload["request_id"] != "req-12345" {
		t.Errorf("expected request_id 'req-12345', got %v", payload["request_id"])
	}
	if payload["service.name"] != "tracepilot-test" {
		t.Errorf("expected service.name 'tracepilot-test', got %v", payload["service.name"])
	}
	if payload["service.version"] != "v1.0.0-test" {
		t.Errorf("expected service.version 'v1.0.0-test', got %v", payload["service.version"])
	}
}

func TestContextHandler_WithoutContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, &Options{
		Level:       slog.LevelInfo,
		ServiceName: "tracepilot-test",
	})

	logger.Info("plain log without context", "key", "val")

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode log json: %v", err)
	}

	if _, ok := payload["trace_id"]; ok {
		t.Errorf("did not expect trace_id in plain log")
	}
	if _, ok := payload["request_id"]; ok {
		t.Errorf("did not expect request_id in plain log")
	}
	if payload["service.name"] != "tracepilot-test" {
		t.Errorf("expected service.name 'tracepilot-test', got %v", payload["service.name"])
	}
	if !strings.Contains(buf.String(), "plain log without context") {
		t.Errorf("log missing message string")
	}
}
