package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// TraceContext holds OpenTelemetry-compatible trace correlation metadata.
type TraceContext struct {
	TraceID    string
	SpanID     string
	RequestID  string
	TraceFlags string
}

type contextKey struct{}

var traceContextKey = contextKey{}

// WithTraceContext returns a copy of parent ctx that stores the given TraceContext.
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey, tc)
}

// FromContext extracts the TraceContext stored in ctx, if present.
func FromContext(ctx context.Context) (TraceContext, bool) {
	if ctx == nil {
		return TraceContext{}, false
	}
	tc, ok := ctx.Value(traceContextKey).(TraceContext)
	return tc, ok
}

var traceparentRegex = regexp.MustCompile(`^([0-9a-f]{2})-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`)

// ParseTraceparent extracts trace correlation metadata according to the W3C Trace Context spec.
// Returns false if the header is absent or invalid.
func ParseTraceparent(header string) (TraceContext, bool) {
	header = strings.TrimSpace(header)
	if len(header) != 55 {
		return TraceContext{}, false
	}
	matches := traceparentRegex.FindStringSubmatch(strings.ToLower(header))
	if len(matches) != 5 {
		return TraceContext{}, false
	}
	version, traceID, spanID, flags := matches[1], matches[2], matches[3], matches[4]
	if version == "ff" || traceID == "00000000000000000000000000000000" || spanID == "0000000000000000" {
		return TraceContext{}, false
	}
	return TraceContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
	}, true
}

// FormatTraceparent formats the trace context into a W3C traceparent header value.
func FormatTraceparent(tc TraceContext) string {
	flags := tc.TraceFlags
	if flags == "" {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", tc.TraceID, tc.SpanID, flags)
}

// NewTraceID returns a random 16-byte hex-encoded string (32 characters).
func NewTraceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NewSpanID returns a random 8-byte hex-encoded string (16 characters).
func NewSpanID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
