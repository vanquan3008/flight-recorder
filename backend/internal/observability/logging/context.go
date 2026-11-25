package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
)

const (
	traceparentVersion        = "00"
	invalidTraceparentVersion = "ff"
	traceparentSeparator      = "-"
	traceparentFieldCount     = 4
	sampledTraceFlags         = "01"

	versionLength    = 2
	traceIDLength    = 32
	spanIDLength     = 16
	traceFlagsLength = 2
)

var (
	zeroTraceID = strings.Repeat("0", traceIDLength)
	zeroSpanID  = strings.Repeat("0", spanIDLength)
)

type TraceContext struct {
	TraceID    string
	SpanID     string
	TraceFlags string
	RequestID  string
}

type traceContextKey struct{}

func WithTraceContext(ctx context.Context, traceContext TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, traceContext)
}

func ContinueTrace(traceparent string) TraceContext {
	if traceContext, ok := parseTraceparent(traceparent); ok {
		return traceContext
	}
	return TraceContext{
		TraceID:    randomNonZeroHex(traceIDLength / 2),
		SpanID:     randomNonZeroHex(spanIDLength / 2),
		TraceFlags: sampledTraceFlags,
	}
}

func (traceContext TraceContext) Traceparent() string {
	fields := []string{traceparentVersion, traceContext.TraceID, traceContext.SpanID, traceContext.TraceFlags}
	return strings.Join(fields, traceparentSeparator)
}

func traceContextFrom(ctx context.Context) TraceContext {
	if ctx == nil {
		return TraceContext{}
	}
	traceContext, _ := ctx.Value(traceContextKey{}).(TraceContext)
	return traceContext
}

func parseTraceparent(header string) (TraceContext, bool) {
	normalized := strings.ToLower(strings.TrimSpace(header))
	fields := strings.Split(normalized, traceparentSeparator)
	if len(fields) != traceparentFieldCount {
		return TraceContext{}, false
	}

	version, traceID, spanID, traceFlags := fields[0], fields[1], fields[2], fields[3]
	validVersion := isLowerHex(version, versionLength) && version != invalidTraceparentVersion
	validTraceID := isLowerHex(traceID, traceIDLength) && traceID != zeroTraceID
	validSpanID := isLowerHex(spanID, spanIDLength) && spanID != zeroSpanID
	validTraceFlags := isLowerHex(traceFlags, traceFlagsLength)
	if !validVersion || !validTraceID || !validSpanID || !validTraceFlags {
		return TraceContext{}, false
	}
	return TraceContext{TraceID: traceID, SpanID: spanID, TraceFlags: traceFlags}, true
}

func randomNonZeroHex(byteCount int) string {
	identifier := make([]byte, byteCount)
	isNonZero := func(value byte) bool { return value != 0 }
	for {
		_, _ = rand.Read(identifier)
		if slices.ContainsFunc(identifier, isNonZero) {
			return hex.EncodeToString(identifier)
		}
	}
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for index := range len(value) {
		character := value[index]
		isDigit := character >= '0' && character <= '9'
		isLetter := character >= 'a' && character <= 'f'
		if !isDigit && !isLetter {
			return false
		}
	}
	return true
}
