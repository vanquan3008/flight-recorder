package otlp

import (
	"bytes"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
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

	first, firstError := normalizer.span(resource, "", nil, "", span)
	second, secondError := normalizer.span(resource, "", nil, "", span)
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
	if _, normalizeError := normalizer.span(resource, "", nil, "", span); normalizeError == nil {
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
	event, normalizeError := normalizer.logRecord(resource, "", nil, "", record)
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

func TestNormalizePreservesTraceStructure(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	startedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "checkout"), stringKeyValue("cloud.region", "ap-southeast-1")}}
	scope := &commonpb.InstrumentationScope{Name: "checkout-sdk", Version: "1.2.3", Attributes: []*commonpb.KeyValue{stringKeyValue("scope.attribute", "scope-value")}}
	span := &tracepb.Span{
		TraceId:                bytes.Repeat([]byte{1}, 16),
		SpanId:                 bytes.Repeat([]byte{2}, 8),
		TraceState:             "vendor=value",
		Flags:                  1,
		Name:                   "POST /checkout",
		Kind:                   tracepb.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano:      uint64(startedAt.UnixNano()),
		EndTimeUnixNano:        uint64(startedAt.Add(time.Second).UnixNano()),
		Status:                 &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "payment failed"},
		Attributes:             []*commonpb.KeyValue{stringKeyValue("http.route", "/checkout")},
		DroppedAttributesCount: 2,
		DroppedEventsCount:     3,
		DroppedLinksCount:      4,
		Events: []*tracepb.Span_Event{{
			TimeUnixNano:           uint64(startedAt.Add(time.Millisecond).UnixNano()),
			Name:                   "payment.failed",
			Attributes:             []*commonpb.KeyValue{stringKeyValue("error.code", "declined")},
			DroppedAttributesCount: 5,
		}},
		Links: []*tracepb.Span_Link{{
			TraceId:                bytes.Repeat([]byte{3}, 16),
			SpanId:                 bytes.Repeat([]byte{4}, 8),
			TraceState:             "linked=value",
			Flags:                  1,
			Attributes:             []*commonpb.KeyValue{stringKeyValue("link.type", "follows_from")},
			DroppedAttributesCount: 6,
		}},
	}
	event, normalizeError := normalizer.span(resource, "https://resource.schema", scope, "https://scope.schema", span)
	if normalizeError != nil {
		t.Fatal(normalizeError)
	}
	if event.Signal != telemetry.SignalTrace || event.TraceState != "vendor=value" || event.TraceFlags != 1 || event.SpanKind != int32(tracepb.Span_SPAN_KIND_SERVER) {
		t.Fatalf("missing trace identity: %+v", event)
	}
	if event.ResourceSchemaURL != "https://resource.schema" || event.ScopeSchemaURL != "https://scope.schema" || event.ResourceAttributes["cloud.region"] != "ap-southeast-1" || event.ScopeAttributes["scope.attribute"] != "scope-value" || event.RecordAttributes["http.route"] != "/checkout" {
		t.Fatalf("missing trace attributes: %+v", event)
	}
	if event.StatusMessage != "payment failed" || event.DroppedAttributes != 2 || event.DroppedEvents != 3 || event.DroppedLinks != 4 || len(event.SpanEvents) != 1 || len(event.SpanLinks) != 1 {
		t.Fatalf("missing span details: %+v", event)
	}
	if event.SpanEvents[0].Attributes["error.code"] != "declined" || event.SpanLinks[0].Attributes["link.type"] != "follows_from" || event.SpanLinks[0].TraceState != "linked=value" {
		t.Fatalf("missing event/link details: %+v", event)
	}
}

func TestNormalizePreservesLogStructure(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	occurredAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	observedAt := occurredAt.Add(time.Second)
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker"), stringKeyValue("host.name", "worker-1")}}
	scope := &commonpb.InstrumentationScope{Name: "zap", Version: "1.0.0", Attributes: []*commonpb.KeyValue{stringKeyValue("scope.attribute", "scope-value")}}
	record := &logspb.LogRecord{
		TimeUnixNano:           uint64(occurredAt.UnixNano()),
		ObservedTimeUnixNano:   uint64(observedAt.UnixNano()),
		TraceId:                bytes.Repeat([]byte{1}, 16),
		SpanId:                 bytes.Repeat([]byte{2}, 8),
		Flags:                  1,
		SeverityNumber:         logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
		SeverityText:           "ERROR",
		Body:                   &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{stringKeyValue("message", "job failed")}}}},
		Attributes:             []*commonpb.KeyValue{stringKeyValue("job.id", "job-1")},
		DroppedAttributesCount: 2,
		EventName:              "job.failure",
	}
	event, normalizeError := normalizer.logRecord(resource, "https://resource.schema", scope, "https://scope.schema", record)
	if normalizeError != nil {
		t.Fatal(normalizeError)
	}
	if event.Signal != telemetry.SignalLog || event.ObservedTimestamp != observedAt || event.TraceFlags != 1 || event.SeverityNumber != int32(logspb.SeverityNumber_SEVERITY_NUMBER_ERROR) || event.LogEventName != "job.failure" {
		t.Fatalf("missing log fields: %+v", event)
	}
	if event.ResourceAttributes["host.name"] != "worker-1" || event.ScopeAttributes["scope.attribute"] != "scope-value" || event.RecordAttributes["job.id"] != "job-1" {
		t.Fatalf("missing log attributes: %+v", event)
	}
	body, ok := event.LogBody.(map[string]any)
	if !ok || body["message"] != "job failed" || event.DroppedAttributes != 2 {
		t.Fatalf("missing log body: %+v", event)
	}
}

func TestNormalizeLogIdentityIncludesEventName(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	occurredAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker")}}
	record := &logspb.LogRecord{
		TimeUnixNano: uint64(occurredAt.UnixNano()),
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "job finished"}},
		EventName:    "job.completed",
	}

	first, firstError := normalizer.logRecord(resource, "", nil, "", record)
	if firstError != nil {
		t.Fatal(firstError)
	}
	retry, retryError := normalizer.logRecord(resource, "", nil, "", record)
	if retryError != nil {
		t.Fatal(retryError)
	}
	if first.ID != retry.ID {
		t.Fatalf("retry changed event ID: %q != %q", first.ID, retry.ID)
	}

	record.EventName = "job.archived"
	second, secondError := normalizer.logRecord(resource, "", nil, "", record)
	if secondError != nil {
		t.Fatal(secondError)
	}
	if first.ID == second.ID {
		t.Fatalf("different event names produced the same ID: %q", first.ID)
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
	if _, normalizeError := normalizer.span(&resourcepb.Resource{}, "", nil, "", span); normalizeError == nil {
		t.Fatal("expected missing service.name to be rejected")
	}
}

func testIngestionConfig() config.IngestionConfig {
	return config.IngestionConfig{
		ProjectID:              "01900000-0000-7000-8000-000000000001",
		EnvironmentID:          "01900000-0000-7000-8000-000000000002",
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

func TestNormalizeSpanIdentityIncludesEnvironment(t *testing.T) {
	startedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "checkout")}}
	span := &tracepb.Span{
		TraceId:           bytes.Repeat([]byte{1}, 16),
		SpanId:            bytes.Repeat([]byte{2}, 8),
		Name:              "POST /checkout",
		StartTimeUnixNano: uint64(startedAt.UnixNano()),
		EndTimeUnixNano:   uint64(startedAt.Add(time.Millisecond).UnixNano()),
	}

	staging := testIngestionConfig()
	staging.EnvironmentID = "01900000-0000-7000-8000-000000000003"
	stagingNormalizer := newNormalizer(staging)
	first, firstError := stagingNormalizer.span(resource, "", nil, "", span)
	if firstError != nil {
		t.Fatal(firstError)
	}
	retry, retryError := stagingNormalizer.span(resource, "", nil, "", span)
	if retryError != nil {
		t.Fatal(retryError)
	}
	if first.ID != retry.ID {
		t.Fatalf("retry changed span event ID: %q != %q", first.ID, retry.ID)
	}
	if first.EnvironmentID != staging.EnvironmentID {
		t.Fatalf("normalized span environment ID = %q, want %q", first.EnvironmentID, staging.EnvironmentID)
	}
	other, otherError := newNormalizer(testIngestionConfig()).span(resource, "", nil, "", span)
	if otherError != nil {
		t.Fatal(otherError)
	}
	if first.ID == other.ID {
		t.Fatalf("same trace and span in different environments share event ID %q", first.ID)
	}
}
