package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	policyID      = "refund-policy"
	modelName     = "refund-demo"
	modelVersion  = "refund-demo-v1"
	promptVersion = "refund-window-v1"
)

type demo struct {
	tracer  trace.Tracer
	logger  *slog.Logger
	lookup  func(context.Context, int32) (refundPolicy, error)
	capture bool
	model   *openAIModel
	logs    *logExporter
}

type refundPolicy struct {
	version int32
	days    int
}

type askResponse struct {
	TraceID       string `json:"trace_id"`
	PolicyVersion int32  `json:"policy_version,omitempty"`
	Answer        string `json:"answer,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (app demo) routes() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("POST /ask", app.ask)
	return router
}

func (app demo) ask(w http.ResponseWriter, r *http.Request) {
	parent := (propagation.TraceContext{}).Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := app.tracer.Start(parent, "POST /ask", trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
		attribute.String("http.request.method", r.Method), attribute.String("http.route", "/ask"),
	))
	defer span.End()
	traceID := span.SpanContext().TraceID().String()
	status, response := app.answer(ctx, http.MaxBytesReader(w, r.Body, 1024))
	response.TraceID = traceID
	span.SetAttributes(attribute.Int("http.response.status_code", status))
	if status >= 500 {
		span.SetStatus(codes.Error, response.Error)
	}
	_, assembly := app.tracer.Start(ctx, "response.assemble", trace.WithAttributes(
		attribute.Int("http.response.status_code", status),
	))
	if response.PolicyVersion > 0 {
		captured := app.capture && response.Answer != ""
		assembly.SetAttributes(attribute.Int("demo.policy.version", int(response.PolicyVersion)))
		assembly.SetAttributes(attribute.Bool("demo.content.captured", captured))
		if captured {
			assembly.SetAttributes(attribute.String("demo.content.output", response.Answer))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	level := slog.LevelInfo
	fields := []slog.Attr{slog.Int("http.response.status_code", status)}
	if response.PolicyVersion > 0 {
		fields = append(fields, slog.Int("demo.policy.version", int(response.PolicyVersion)))
	}
	if status >= 500 {
		level = slog.LevelWarn
		fields = append(fields, slog.String("error.type", strconv.Itoa(status)))
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		assembly.SetStatus(codes.Error, "response write failed")
		span.SetStatus(codes.Error, "response write failed")
		level = slog.LevelWarn
		fields = append(fields, slog.Bool("demo.response.write_failed", true))
	}
	assembly.End()
	app.recordLog(ctx, level, "request.completed", fields...)
}

func (app demo) answer(ctx context.Context, body io.Reader) (int, askResponse) {
	var input *struct {
		PolicyVersion int32 `json:"policy_version"`
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input == nil || input.PolicyVersion < 0 {
		return http.StatusBadRequest, askResponse{Error: "expected a JSON object with optional nonnegative policy_version"}
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return http.StatusBadRequest, askResponse{Error: "expected one JSON object"}
	}

	policy, err := app.retrieve(ctx, input.PolicyVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound, askResponse{Error: "policy version not found"}
	}
	if err != nil {
		return http.StatusInternalServerError, askResponse{Error: "policy retrieval failed"}
	}
	answer, err := app.generate(ctx, policy)
	if errors.Is(err, errModelTimeout) {
		return http.StatusGatewayTimeout, askResponse{PolicyVersion: policy.version, Error: errModelTimeout.Error()}
	}
	if err != nil {
		return http.StatusBadGateway, askResponse{PolicyVersion: policy.version, Error: errModelFailed.Error()}
	}
	return http.StatusOK, askResponse{PolicyVersion: policy.version, Answer: answer}
}

func (app demo) retrieve(ctx context.Context, version int32) (refundPolicy, error) {
	ctx, retrieval := app.tracer.Start(ctx, "policy.retrieve")
	defer retrieval.End()
	databaseCtx, database := app.tracer.Start(ctx, "SELECT refund_policies", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(
		attribute.String("db.system.name", "postgresql"), attribute.String("db.system", "postgresql"),
		attribute.String("db.operation.name", "SELECT"),
	))
	policy, err := app.lookup(databaseCtx, version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		database.SetStatus(codes.Error, "database query failed")
		retrieval.SetStatus(codes.Error, "policy retrieval failed")
	}
	database.End()
	if err == nil {
		retrieval.SetAttributes(attribute.String("demo.policy.id", policyID), attribute.Int("demo.policy.version", int(policy.version)))
		app.recordLog(ctx, slog.LevelInfo, "policy.retrieved",
			slog.String("demo.policy.id", policyID), slog.Int("demo.policy.version", int(policy.version)))
	} else {
		errorType := "database_error"
		if errors.Is(err, pgx.ErrNoRows) {
			errorType = "policy_not_found"
		}
		app.recordLog(ctx, slog.LevelWarn, "policy.lookup_failed", slog.String("error.type", errorType))
	}
	return policy, err
}

func (app demo) generate(ctx context.Context, policy refundPolicy) (string, error) {
	provider, requestedModel, operation, kind := serviceName, modelName, "chat "+modelName, trace.SpanKindInternal
	if app.model != nil {
		provider, requestedModel, operation, kind = "openai", app.model.name, "chat openai", trace.SpanKindClient
	}
	ctx, span := app.tracer.Start(ctx, operation, trace.WithSpanKind(kind), trace.WithAttributes(
		attribute.String("gen_ai.operation.name", "chat"), attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.request.model", requestedModel),
		attribute.String("demo.prompt.version", promptVersion), attribute.Int("demo.policy.version", int(policy.version)),
		attribute.Bool("demo.content.captured", app.capture),
	))
	defer span.End()
	const instructions = "Answer the refund-window question using the supplied policy."
	input := fmt.Sprintf("You can request a refund within %d days.", policy.days)
	if app.capture {
		span.SetAttributes(attribute.StringSlice("demo.content.input", []string{instructions, input}))
	}
	// The stub returns the supplied policy sentence so tests can reproduce stale answers.
	result := modelResult{answer: input, model: modelVersion}
	var err error
	if app.model != nil {
		result, err = app.model.generate(ctx, instructions, input)
	}
	if result.model != "" {
		span.SetAttributes(attribute.String("gen_ai.response.model", result.model))
	}
	if result.usage != nil {
		if result.usage.InputTokens != nil {
			span.SetAttributes(attribute.Int64("gen_ai.usage.input_tokens", *result.usage.InputTokens))
		}
		if result.usage.OutputTokens != nil {
			span.SetAttributes(attribute.Int64("gen_ai.usage.output_tokens", *result.usage.OutputTokens))
		}
	}
	fields := []slog.Attr{
		slog.String("gen_ai.provider.name", provider), slog.String("gen_ai.request.model", requestedModel),
		slog.String("demo.prompt.version", promptVersion), slog.Int("demo.policy.version", int(policy.version)),
	}
	if result.model != "" {
		fields = append(fields, slog.String("gen_ai.response.model", result.model))
	}
	if err != nil {
		errorType := "provider_error"
		if errors.Is(err, errModelTimeout) {
			errorType = "timeout"
		}
		span.SetAttributes(attribute.String("error.type", errorType))
		span.SetStatus(codes.Error, err.Error())
		app.recordLog(ctx, slog.LevelWarn, "generation.failed", append(fields, slog.String("error.type", errorType))...)
		return "", err
	}
	if app.capture {
		span.SetAttributes(attribute.String("demo.content.output", result.answer))
	}
	app.recordLog(ctx, slog.LevelInfo, "generation.completed", fields...)
	return result.answer, nil
}

func (app demo) recordLog(ctx context.Context, level slog.Level, message string, fields ...slog.Attr) {
	span := trace.SpanContextFromContext(ctx)
	traceID, spanID := span.TraceID().String(), span.SpanID().String()
	app.logger.LogAttrs(ctx, level, message, append(fields, slog.String("trace_id", traceID), slog.String("span_id", spanID))...)
	if app.logs == nil || !span.IsValid() {
		return
	}
	number := 9
	if level == slog.LevelWarn {
		number = 13
	}
	record := logRecord{Time: strconv.FormatInt(time.Now().UnixNano(), 10), SeverityNumber: number,
		SeverityText: level.String(), TraceID: traceID, SpanID: spanID, Body: logValue{String: message}}
	for _, field := range fields {
		var value logValue
		switch field.Value.Kind() {
		case slog.KindString:
			value.String = field.Value.String()
		case slog.KindInt64:
			value.Int = strconv.FormatInt(field.Value.Int64(), 10)
		case slog.KindBool:
			truth := field.Value.Bool()
			value.Bool = &truth
		default:
			continue
		}
		record.Attributes = append(record.Attributes, logAttribute{Key: field.Key, Value: value})
	}
	app.logs.enqueue(record)
}
