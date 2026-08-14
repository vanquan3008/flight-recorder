package otlp

import (
	"bytes"
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/config"
	"github.com/flight-observability/flight/internal/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestNormalizeTraceRedactsBeforeReturningEvent(t *testing.T) {
	configuration := testIngestionConfig()
	normalizer := newNormalizer(configuration)
	startedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	span := &tracepb.Span{
		TraceId:           bytes.Repeat([]byte{1}, 16),
		SpanId:            bytes.Repeat([]byte{2}, 8),
		Name:              "GET /error",
		Kind:              tracepb.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: uint64(startedAt.UnixNano()),
		EndTimeUnixNano:   uint64(startedAt.Add(10 * time.Millisecond).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			stringKeyValue("http.request.header.authorization", "Bearer production-secret"),
			intKeyValue("http.response.status_code", 500),
			stringKeyValue("service.name", "span-must-not-override-resource"),
		},
		Events: []*tracepb.Span_Event{{
			Name: "exception",
			Attributes: []*commonpb.KeyValue{
				stringKeyValue("exception.type", "SyntheticError"),
				stringKeyValue("exception.message", "boom"),
			},
		}},
	}
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "checkout")}}

	first, firstError := normalizer.span(resource, nil, span)
	second, secondError := normalizer.span(resource, nil, span)
	if firstError != nil || secondError != nil {
		t.Fatalf("normalize errors: %v, %v", firstError, secondError)
	}
	if first.Type != telemetry.EventException || first.HTTPStatus != 500 || first.ErrorType != "SyntheticError" {
		t.Fatalf("unexpected event: %+v", first)
	}
	if first.ServiceName != "checkout" || first.Attributes["service.name"] != "checkout" {
		t.Fatalf("record attributes overrode resource identity: %+v", first)
	}
	if first.Attributes["http.request.header.authorization"] != "[REDACTED]" {
		t.Fatalf("secret was not redacted: %#v", first.Attributes)
	}
	if first.ID != second.ID {
		t.Fatalf("retry changed event ID: %q != %q", first.ID, second.ID)
	}
}

func TestNormalizeRejectsInvalidHTTPStatus(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	startedAt := time.Now().UTC()
	span := &tracepb.Span{
		TraceId:           bytes.Repeat([]byte{1}, 16),
		SpanId:            bytes.Repeat([]byte{2}, 8),
		StartTimeUnixNano: uint64(startedAt.UnixNano()),
		EndTimeUnixNano:   uint64(startedAt.Add(time.Millisecond).UnixNano()),
		Attributes:        []*commonpb.KeyValue{intKeyValue("http.response.status_code", 999)},
	}
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "checkout")}}
	if _, normalizeError := normalizer.span(resource, nil, span); normalizeError == nil {
		t.Fatal("expected invalid HTTP status to be rejected")
	}
}

func TestNormalizeLogRecord(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker")}}
	record := &logspb.LogRecord{
		TimeUnixNano:   uint64(time.Now().UTC().UnixNano()),
		SeverityText:   "ERROR",
		SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
		Body:           &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "job failed"}},
		Attributes:     []*commonpb.KeyValue{stringKeyValue("api.token", "secret")},
	}
	event, normalizeError := normalizer.logRecord(resource, nil, record)
	if normalizeError != nil {
		t.Fatal(normalizeError)
	}
	if event.Type != telemetry.EventLog || event.Severity != "ERROR" || event.ErrorMessage != "job failed" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Attributes["api.token"] != "[REDACTED]" {
		t.Fatalf("log secret was not redacted: %#v", event.Attributes)
	}
}

func TestNormalizeRejectsMissingService(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	startedAt := time.Now().UTC()
	span := &tracepb.Span{
		TraceId:           bytes.Repeat([]byte{1}, 16),
		SpanId:            bytes.Repeat([]byte{2}, 8),
		StartTimeUnixNano: uint64(startedAt.UnixNano()),
		EndTimeUnixNano:   uint64(startedAt.Add(time.Millisecond).UnixNano()),
	}
	if _, normalizeError := normalizer.span(&resourcepb.Resource{}, nil, span); normalizeError == nil {
		t.Fatal("expected missing service.name to be rejected")
	}
}

func testIngestionConfig() config.IngestionConfig {
	return config.IngestionConfig{
		ProjectID:              "01900000-0000-7000-8000-000000000001",
		EnvironmentName:        "development",
		MaxRecordsPerRequest:   100,
		MaxAttributesPerRecord: 16,
		MaxAttributeValueBytes: 1024,
		MaxAttributeDepth:      4,
	}
}

func stringKeyValue(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func intKeyValue(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}}
}
