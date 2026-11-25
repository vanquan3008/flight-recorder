package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestStalePolicyTracePreservesParentAndEvidence(t *testing.T) {
	const upstreamTrace = "0123456789abcdef0123456789abcdef"
	const upstreamSpan = "0123456789abcdef"
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	app := demo{
		tracer: provider.Tracer(serviceName), logger: slog.New(slog.DiscardHandler),
		lookup: func(ctx context.Context, version int32) (refundPolicy, error) {
			if version != 1 || trace.SpanContextFromContext(ctx).TraceID().String() != upstreamTrace {
				t.Fatalf("unexpected lookup version %d or trace context", version)
			}
			return refundPolicy{version: 1, days: 14}, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"policy_version":1}`))
	request.Header.Set("traceparent", "00-"+upstreamTrace+"-"+upstreamSpan+"-01")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	result := readAnswer(t, response)
	if response.Code != http.StatusOK || result.TraceID != upstreamTrace || result.PolicyVersion != 1 || result.Answer != "You can request a refund within 14 days." {
		t.Fatalf("unexpected HTTP response %d, %+v", response.Code, result)
	}
	spans := make(map[string]sdktrace.ReadOnlySpan)
	for _, span := range recorder.Ended() {
		if span.SpanContext().TraceID().String() != result.TraceID {
			t.Fatalf("span %q has a different trace ID", span.Name())
		}
		spans[span.Name()] = span
	}
	root, retrieval, database := spans["POST /ask"], spans["policy.retrieve"], spans["SELECT refund_policies"]
	generation, assembly := spans["chat "+modelName], spans["response.assemble"]
	if len(spans) != 5 || root == nil || retrieval == nil || database == nil || generation == nil || assembly == nil {
		t.Fatalf("unexpected span tree: %v", spans)
	}
	if root.Parent().SpanID().String() != upstreamSpan || root.SpanKind() != trace.SpanKindServer || database.SpanKind() != trace.SpanKindClient {
		t.Fatal("expected the propagated parent and server/client span kinds")
	}
	for _, child := range []sdktrace.ReadOnlySpan{retrieval, generation, assembly} {
		if child.Parent().SpanID() != root.SpanContext().SpanID() {
			t.Errorf("%q is not linked to the HTTP server span", child.Name())
		}
	}
	if database.Parent().SpanID() != retrieval.SpanContext().SpanID() {
		t.Fatal("PostgreSQL span is not linked to retrieval")
	}
	if !hasAttribute(retrieval, attribute.Int("demo.policy.version", 1)) ||
		!hasAttribute(generation, attribute.String("gen_ai.response.model", modelVersion)) ||
		!hasAttribute(assembly, attribute.Int("http.response.status_code", http.StatusOK)) {
		t.Fatal("missing policy, model, or response evidence")
	}
}

func TestFailedPolicyLookupProducesTraceWithoutLeakingDriverError(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	var logs bytes.Buffer
	app := demo{
		tracer: provider.Tracer(serviceName), logger: slog.New(slog.NewJSONHandler(&logs, nil)),
		lookup: func(context.Context, int32) (refundPolicy, error) { return refundPolicy{}, errors.New("driver detail") },
	}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{}`)))
	result := readAnswer(t, response)
	if response.Code != http.StatusInternalServerError || result.TraceID == "" || result.Error != "policy retrieval failed" {
		t.Fatalf("unexpected HTTP response %d, %+v", response.Code, result)
	}
	if !strings.Contains(logs.String(), result.TraceID) || strings.Contains(logs.String(), "driver detail") {
		t.Fatalf("unexpected request log: %s", logs.String())
	}
	failed := 0
	for _, span := range recorder.Ended() {
		if span.Name() == "chat "+modelName || strings.Contains(span.Status().Description, "driver detail") {
			t.Fatalf("unexpected span after lookup failure: %q, %+v", span.Name(), span.Status())
		}
		if span.Status().Code == codes.Error {
			failed++
		}
	}
	if failed != 3 {
		t.Fatalf("expected database, retrieval, and HTTP spans to be errors, got %d", failed)
	}
}

func TestDemoContentRequiresOptInAndLocalCollector(t *testing.T) {
	for _, test := range []struct {
		name, value, endpoint string
		want, wantError       bool
	}{
		{name: "default off", endpoint: demoCollectorEndpoint},
		{name: "invalid setting", value: "sometimes", endpoint: demoCollectorEndpoint, wantError: true},
		{name: "direct to TracePilot", value: "true", endpoint: "http://127.0.0.1:4318/v1/traces", wantError: true},
		{name: "local Collector", value: "true", endpoint: demoCollectorEndpoint, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture, err := contentCapture(test.value, test.endpoint)
			if capture != test.want || (err != nil) != test.wantError {
				t.Fatalf("capture = %v, error = %v", capture, err)
			}
			if err != nil {
				return
			}
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			app := demo{
				tracer: provider.Tracer(serviceName), logger: slog.New(slog.DiscardHandler), capture: capture,
				lookup: func(context.Context, int32) (refundPolicy, error) { return refundPolicy{version: 1, days: 14}, nil },
			}
			response := httptest.NewRecorder()
			app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"policy_version":1}`)))
			result := readAnswer(t, response)
			if response.Code != http.StatusOK || result.Answer == "" {
				t.Fatalf("unexpected HTTP response %d, %+v", response.Code, result)
			}
			contentSpans := 0
			for _, span := range recorder.Ended() {
				if span.Name() != "chat "+modelName && span.Name() != "response.assemble" {
					continue
				}
				contentSpans++
				if !hasAttribute(span, attribute.Bool("demo.content.captured", test.want)) ||
					hasAttribute(span, attribute.String("demo.content.output", result.Answer)) != test.want {
					t.Errorf("unexpected output capture on %q", span.Name())
				}
				if span.Name() == "chat "+modelName &&
					hasAttribute(span, attribute.StringSlice("demo.content.input", []string{"Answer the refund-window question using the supplied policy.", result.Answer})) != test.want {
					t.Error("unexpected generation input capture")
				}
			}
			if contentSpans != 2 {
				t.Fatalf("expected generation and response spans, got %d", contentSpans)
			}
		})
	}
}

func TestPostgreSQLPolicyAndOTLPExport(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 and TRACEPILOT_DATABASE_URL for a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	pool, err := openDemoDatabase(ctx, os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := policyLookup(pool)(ctx, 99); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected missing policy, got %v", err)
	}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Input != "You can request a refund within 30 days." {
			t.Error("model did not receive the latest policy from PostgreSQL")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		io.WriteString(w, `{"status":"completed","model":"reported-model","usage":{"input_tokens":21,"output_tokens":7},"output":[{"type":"message","content":[{"type":"output_text","text":"Refund requests are accepted for 30 days."}]}]}`)
	}))
	defer modelServer.Close()

	exports := make(chan *collectorpb.ExportTraceServiceRequest, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		batch := &collectorpb.ExportTraceServiceRequest{}
		if err != nil || proto.Unmarshal(body, batch) != nil || r.Method != http.MethodPost || r.URL.Path != "/v1/traces" ||
			r.Header.Get("Authorization") != "Bearer test-only" || r.Header.Get("X-Environment") != "dev" {
			t.Error("invalid OTLP request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		exports <- batch
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(receiver.URL+"/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{"Authorization": "Bearer test-only", "X-Environment": "dev"}),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression))
	if err != nil {
		t.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(
		resource.NewSchemaless(attribute.String("service.name", serviceName))))
	defer func() {
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	app := demo{
		tracer: provider.Tracer(serviceName), logger: slog.New(slog.DiscardHandler), lookup: policyLookup(pool),
		model: &openAIModel{name: "requested-model", endpoint: modelServer.URL, client: modelServer.Client()},
	}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{}`)).WithContext(ctx))
	result := readAnswer(t, response)
	if response.Code != http.StatusOK || result.PolicyVersion != 2 || result.Answer != "Refund requests are accepted for 30 days." {
		t.Fatalf("unexpected HTTP response %d, %+v", response.Code, result)
	}
	if err := provider.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case batch := <-exports:
		spanCount, generationFound := 0, false
		for _, resourceSpans := range batch.GetResourceSpans() {
			for _, scope := range resourceSpans.GetScopeSpans() {
				for _, span := range scope.GetSpans() {
					spanCount++
					if len(span.GetTraceId()) != 16 || result.TraceID != trace.TraceID(span.GetTraceId()).String() {
						t.Fatal("exported span has the wrong trace ID")
					}
					generationFound = generationFound || span.GetName() == "chat openai"
				}
			}
		}
		if spanCount != 5 || !generationFound {
			t.Fatal("expected five exported spans correlated with the response")
		}
	case <-ctx.Done():
		t.Fatal("did not receive an OTLP export")
	}
}

func readAnswer(t *testing.T, response *httptest.ResponseRecorder) askResponse {
	t.Helper()
	var result askResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func hasAttribute(span sdktrace.ReadOnlySpan, expected attribute.KeyValue) bool {
	for _, actual := range span.Attributes() {
		if actual.Key == expected.Key && actual.Value.String() == expected.Value.String() {
			return true
		}
	}
	return false
}
