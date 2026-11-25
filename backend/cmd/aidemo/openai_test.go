package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestOpenAISandboxTraceUsesReportedModelAndTokens(t *testing.T) {
	const input = "You can request a refund within 14 days."
	const instructions = "Answer the refund-window question using the supplied policy."
	const output = "The refund window is 14 days."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-sandbox-key" {
			t.Error("unexpected OpenAI HTTP request")
		}
		var body struct {
			Model        string `json:"model"`
			Instructions string `json:"instructions"`
			Input        string `json:"input"`
			Store        *bool  `json:"store"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "requested-model" ||
			body.Instructions != instructions || body.Input != input || body.Store == nil || *body.Store {
			t.Error("unexpected OpenAI request body")
		}
		fmt.Fprint(w, `{"model":"reported-model","status":"completed","usage":{"input_tokens":21,"output_tokens":9},"output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"The refund window "},{"type":"output_text","text":"is 14 days."}]}]}`)
	}))
	defer server.Close()

	app, recorder := openAITestDemo(server.URL+"/v1/responses", server.Client())
	app.capture = true
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"policy_version":1}`)))
	result := readAnswer(t, response)
	if response.Code != http.StatusOK || result.Answer != output || result.PolicyVersion != 1 {
		t.Fatalf("unexpected HTTP response %d, %+v", response.Code, result)
	}
	spans := openAITestSpans(t, recorder, result.TraceID)
	generation := spans["chat openai"]
	if generation.SpanKind() != trace.SpanKindClient || generation.Parent().SpanID() != spans["POST /ask"].SpanContext().SpanID() {
		t.Fatal("expected an outbound generation span under the HTTP request")
	}
	for _, attr := range []attribute.KeyValue{
		attribute.String("gen_ai.request.model", "requested-model"),
		attribute.String("gen_ai.response.model", "reported-model"),
		attribute.Int64("gen_ai.usage.input_tokens", 21),
		attribute.Int64("gen_ai.usage.output_tokens", 9),
		attribute.StringSlice("demo.content.input", []string{instructions, input}),
		attribute.String("demo.content.output", output),
	} {
		if !hasAttribute(generation, attr) {
			t.Errorf("generation missing %s", attr.Key)
		}
	}
	if !hasAttribute(spans["response.assemble"], attribute.String("demo.content.output", output)) {
		t.Fatal("response span missing the final answer")
	}
}

func TestOpenAISandboxRecordsUpstreamErrorsWithoutRawResponse(t *testing.T) {
	for _, test := range []struct {
		name, body, errorType  string
		providerStatus, status int
	}{
		{"timeout", "", "timeout", http.StatusGatewayTimeout, http.StatusGatewayTimeout},
		{"provider error", `{"error":"test-private-provider-detail"}`, "provider_error", http.StatusUnauthorized, http.StatusBadGateway},
		{"incomplete", `{"status":"incomplete","model":"reported-model","usage":{"input_tokens":21,"output_tokens":3}}`, "provider_error", http.StatusOK, http.StatusBadGateway},
		{"refusal", `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"test-private-provider-detail"}]}]}`, "provider_error", http.StatusOK, http.StatusBadGateway},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.providerStatus)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			app, recorder := openAITestDemo(server.URL, server.Client())
			var logs bytes.Buffer
			app.logger = slog.New(slog.NewJSONHandler(&logs, nil))
			response := httptest.NewRecorder()
			app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{}`)))
			result := readAnswer(t, response)
			if response.Code != test.status || result.PolicyVersion != 1 || result.Answer != "" || result.Error == "" {
				t.Fatalf("unexpected HTTP response %d, %+v", response.Code, result)
			}
			spans := openAITestSpans(t, recorder, result.TraceID)
			generation := spans["chat openai"]
			if generation.Status().Code != codes.Error || spans["POST /ask"].Status().Code != codes.Error ||
				!hasAttribute(generation, attribute.String("error.type", test.errorType)) ||
				!hasAttribute(spans["response.assemble"], attribute.Int("http.response.status_code", test.status)) {
				t.Fatal("missing error status or evidence")
			}
			if strings.Contains(logs.String()+result.Error+generation.Status().Description, "test-private-provider-detail") {
				t.Fatal("provider response leaked into trace, logs, or API error")
			}
			foundWarning := false
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, `"msg":"generation.failed"`) &&
					strings.Contains(line, `"level":"WARN"`) &&
					strings.Contains(line, `"error.type":"`+test.errorType+`"`) &&
					strings.Contains(line, generation.SpanContext().SpanID().String()) {
					foundWarning = true
				}
			}
			if !foundWarning {
				t.Fatal("missing normalized generation warning on the generation span")
			}
			if test.name == "incomplete" && !hasAttribute(generation, attribute.Int64("gen_ai.usage.output_tokens", 3)) {
				t.Fatal("missing usage reported by the incomplete response")
			}
		})
	}
}

func TestOpenAIClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(250 * time.Millisecond):
		}
	}))
	defer server.Close()
	model := openAIModel{name: "requested-model", endpoint: server.URL, client: &http.Client{Timeout: 10 * time.Millisecond}}
	if _, err := model.generate(t.Context(), "instructions", "input"); err != errModelTimeout {
		t.Fatalf("generate returned %v, want normalized timeout", err)
	}
}

func TestOpenAIRequiresExplicitModelAndCredentials(t *testing.T) {
	if model, err := configuredOpenAIModel("", "unused-key"); model != nil || err != nil {
		t.Fatal("an API key alone enabled OpenAI")
	}
	if model, err := configuredOpenAIModel("requested-model", ""); model != nil || err == nil {
		t.Fatal("expected a configured model without credentials to fail")
	}
	if model, err := configuredOpenAIModel("requested-model", "test-sandbox-key"); err != nil || model == nil || model.endpoint != openAIResponsesEndpoint {
		t.Fatalf("expected default OpenAI Responses endpoint, got %v", err)
	}
}

func openAITestDemo(endpoint string, client *http.Client) (demo, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return demo{
		tracer: provider.Tracer(serviceName), logger: slog.New(slog.DiscardHandler),
		lookup: func(context.Context, int32) (refundPolicy, error) { return refundPolicy{version: 1, days: 14}, nil },
		model:  &openAIModel{name: "requested-model", apiKey: "test-sandbox-key", endpoint: endpoint, client: client},
	}, recorder
}

func openAITestSpans(t *testing.T, recorder *tracetest.SpanRecorder, traceID string) map[string]sdktrace.ReadOnlySpan {
	t.Helper()
	spans := make(map[string]sdktrace.ReadOnlySpan)
	for _, span := range recorder.Ended() {
		if span.SpanContext().TraceID().String() != traceID {
			t.Fatalf("span %q has a different trace ID", span.Name())
		}
		spans[span.Name()] = span
	}
	if len(spans) != 5 || spans["chat openai"] == nil || spans["POST /ask"] == nil || spans["response.assemble"] == nil {
		t.Fatal("missing request or generation evidence")
	}
	return spans
}
