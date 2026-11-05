package logging

import (
	"context"
	"io"
	"log/slog"
)

// Options specifies configuration for the production context logger.
type Options struct {
	Level          slog.Leveler
	ServiceName    string
	ServiceVersion string
	AddSource      bool
}

// ContextHandler wraps an underlying slog.Handler to inject OpenTelemetry-compatible
// trace_id, span_id, request_id, and service metadata automatically into records.
type ContextHandler struct {
	inner          slog.Handler
	serviceName    string
	serviceVersion string
}

// NewHandler constructs a ContextHandler writing JSON to w with the provided options.
func NewHandler(w io.Writer, opts *Options) *ContextHandler {
	if opts == nil {
		opts = &Options{}
	}
	level := opts.Level
	if level == nil {
		level = slog.LevelInfo
	}
	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.AddSource,
	})
	return &ContextHandler{
		inner:          jsonHandler,
		serviceName:    opts.ServiceName,
		serviceVersion: opts.ServiceVersion,
	}
}

// NewLogger constructs an *slog.Logger configured with ContextHandler.
func NewLogger(w io.Writer, opts *Options) *slog.Logger {
	return slog.New(NewHandler(w, opts))
}

// Enabled reports whether the handler emits log records at the given level.
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle injects trace correlation and service attributes before delegating to the inner handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	var attrs []slog.Attr

	if tc, ok := FromContext(ctx); ok {
		if tc.TraceID != "" && !hasAttr(r, "trace_id") {
			attrs = append(attrs, slog.String("trace_id", tc.TraceID))
		}
		if tc.SpanID != "" && !hasAttr(r, "span_id") {
			attrs = append(attrs, slog.String("span_id", tc.SpanID))
		}
		if tc.RequestID != "" && !hasAttr(r, "request_id") {
			attrs = append(attrs, slog.String("request_id", tc.RequestID))
		}
	}

	if h.serviceName != "" && !hasAttr(r, "service.name") {
		attrs = append(attrs, slog.String("service.name", h.serviceName))
	}
	if h.serviceVersion != "" && !hasAttr(r, "service.version") {
		attrs = append(attrs, slog.String("service.version", h.serviceVersion))
	}

	if len(attrs) > 0 {
		r.AddAttrs(attrs...)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler with the given attributes added.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{
		inner:          h.inner.WithAttrs(attrs),
		serviceName:    h.serviceName,
		serviceVersion: h.serviceVersion,
	}
}

// WithGroup returns a new ContextHandler with a qualified group name.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{
		inner:          h.inner.WithGroup(name),
		serviceName:    h.serviceName,
		serviceVersion: h.serviceVersion,
	}
}

func hasAttr(r slog.Record, key string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = true
			return false
		}
		return true
	})
	return found
}
