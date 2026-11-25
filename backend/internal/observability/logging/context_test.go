package logging

import (
	"context"
	"testing"
)

const (
	sampleTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	sampleSpanID      = "00f067aa0ba902b7"
	sampleTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
)

func TestParseTraceparent(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   TraceContext
		wantOK bool
	}{
		{
			name:   "standard header",
			header: sampleTraceparent,
			want:   TraceContext{TraceID: sampleTraceID, SpanID: sampleSpanID, TraceFlags: "01"},
			wantOK: true,
		},
		{
			name:   "surrounding whitespace and uppercase hex",
			header: "  00-4BF92F3577B34DA6A3CE929D0E0E4736-00F067AA0BA902B7-00  ",
			want:   TraceContext{TraceID: sampleTraceID, SpanID: sampleSpanID, TraceFlags: "00"},
			wantOK: true,
		},
		{name: "empty header", header: ""},
		{name: "missing trace flags", header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7"},
		{name: "extra field", header: sampleTraceparent + "-01"},
		{name: "forbidden version ff", header: "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{name: "non-hex version", header: "0g-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{name: "all-zero trace ID", header: "00-00000000000000000000000000000000-00f067aa0ba902b7-01"},
		{name: "all-zero span ID", header: "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01"},
		{name: "non-hex trace ID", header: "00-4bf92f3577b34da6a3ce929d0e0e473g-00f067aa0ba902b7-01"},
		{name: "short span ID", header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902-01"},
		{name: "non-hex trace flags", header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0z"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseTraceparent(test.header)
			if ok != test.wantOK {
				t.Fatalf("parseTraceparent(%q) ok = %v, want %v", test.header, ok, test.wantOK)
			}
			if got != test.want {
				t.Fatalf("parseTraceparent(%q) = %+v, want %+v", test.header, got, test.want)
			}
		})
	}
}

func TestContinueTraceKeepsAValidIncomingTrace(t *testing.T) {
	traceContext := ContinueTrace(sampleTraceparent)

	if got := traceContext.Traceparent(); got != sampleTraceparent {
		t.Fatalf("Traceparent() = %q, want %q", got, sampleTraceparent)
	}
}

func TestContinueTraceStartsASampledTraceWhenTheHeaderIsInvalid(t *testing.T) {
	for _, header := range []string{"", "not-a-traceparent", "00-00000000000000000000000000000000-00f067aa0ba902b7-01"} {
		traceContext := ContinueTrace(header)

		parsed, ok := parseTraceparent(traceContext.Traceparent())
		if !ok || parsed != traceContext {
			t.Fatalf("ContinueTrace(%q) = %+v, which is not a valid traceparent", header, traceContext)
		}
		if traceContext.TraceFlags != sampledTraceFlags {
			t.Fatalf("ContinueTrace(%q) flags = %q, want %q", header, traceContext.TraceFlags, sampledTraceFlags)
		}
	}

	if first, second := ContinueTrace(""), ContinueTrace(""); first.TraceID == second.TraceID || first.SpanID == second.SpanID {
		t.Fatalf("two new traces share identifiers: %+v and %+v", first, second)
	}
}

func TestTraceContextTravelsThroughContext(t *testing.T) {
	stored := TraceContext{TraceID: sampleTraceID, SpanID: sampleSpanID, TraceFlags: "01", RequestID: "req-1"}

	if got := traceContextFrom(WithTraceContext(context.Background(), stored)); got != stored {
		t.Fatalf("traceContextFrom() = %+v, want %+v", got, stored)
	}
	if got := traceContextFrom(context.Background()); got != (TraceContext{}) {
		t.Fatalf("traceContextFrom(empty) = %+v, want the zero value", got)
	}
	var missingContext context.Context
	if got := traceContextFrom(missingContext); got != (TraceContext{}) {
		t.Fatalf("traceContextFrom(nil) = %+v, want the zero value", got)
	}
}
