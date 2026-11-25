package logging

import (
	"context"
	"io"
	"log/slog"
)

const (
	traceIDKey        = "trace_id"
	spanIDKey         = "span_id"
	requestIDKey      = "request_id"
	serviceNameKey    = "service.name"
	serviceVersionKey = "service.version"
)

type Options struct {
	Level          slog.Leveler
	ServiceName    string
	ServiceVersion string
}

type contextHandler struct {
	inner          slog.Handler
	serviceName    string
	serviceVersion string
}

func NewLogger(output io.Writer, options *Options) *slog.Logger {
	if options == nil {
		options = &Options{}
	}
	level := options.Level
	if level == nil {
		level = slog.LevelInfo
	}
	return slog.New(&contextHandler{
		inner:          slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}),
		serviceName:    options.ServiceName,
		serviceVersion: options.ServiceVersion,
	})
}

func (handler *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.inner.Enabled(ctx, level)
}

func (handler *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	traceContext := traceContextFrom(ctx)
	correlation := []slog.Attr{
		slog.String(traceIDKey, traceContext.TraceID),
		slog.String(spanIDKey, traceContext.SpanID),
		slog.String(requestIDKey, traceContext.RequestID),
		slog.String(serviceNameKey, handler.serviceName),
		slog.String(serviceVersionKey, handler.serviceVersion),
	}
	for _, attribute := range correlation {
		if attribute.Value.String() == "" || recordHasAttribute(record, attribute.Key) {
			continue
		}
		record.AddAttrs(attribute)
	}
	return handler.inner.Handle(ctx, record)
}

func (handler *contextHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	return handler.wrap(handler.inner.WithAttrs(attributes))
}

func (handler *contextHandler) WithGroup(name string) slog.Handler {
	return handler.wrap(handler.inner.WithGroup(name))
}

func (handler *contextHandler) wrap(inner slog.Handler) *contextHandler {
	return &contextHandler{
		inner:          inner,
		serviceName:    handler.serviceName,
		serviceVersion: handler.serviceVersion,
	}
}

func recordHasAttribute(record slog.Record, key string) bool {
	found := false
	record.Attrs(func(attribute slog.Attr) bool {
		found = attribute.Key == key
		return !found
	})
	return found
}
