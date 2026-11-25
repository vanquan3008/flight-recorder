package normalize

import (
	"reflect"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestLogIDsIncludeEnvironment(t *testing.T) {
	configuration := normalizetest.Config()
	first := New(configuration)
	configuration.EnvironmentID = "01900000-0000-7000-8000-000000000003"
	second := New(configuration)
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}}
	const timestamp = uint64(1791507600123456789)
	record := &logspb.LogRecord{TimeUnixNano: timestamp}
	firstLog, err := first.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	if firstLog.EnvironmentID != normalizetest.Config().EnvironmentID {
		t.Fatalf("log environment ID = %q", firstLog.EnvironmentID)
	}
	secondLog, err := second.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	if firstLog.ID == secondLog.ID {
		t.Fatal("logs in different environments received the same primary key")
	}
}

func TestLogIdentityPreservesNanosecondsAndRetries(t *testing.T) {
	normalizer := New(normalizetest.Config())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}}
	at := time.Date(2026, 11, 16, 12, 0, 0, 123100000, time.UTC)
	record := &logspb.LogRecord{TimeUnixNano: uint64(at.UnixNano()), Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "job completed"}}}
	first, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	record.TimeUnixNano++
	next, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != retry.ID || first.ID == next.ID {
		t.Fatalf("log IDs: first=%s retry=%s next nanosecond=%s", first.ID, retry.ID, next.ID)
	}
}

func TestLogIdentityKeepsAttributeOrigins(t *testing.T) {
	normalizer := New(normalizetest.Config())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker"), normalizetest.StringKeyValue("host.name", "host-1")}}
	record := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano())}
	record.Attributes = append(record.Attributes, normalizetest.StringKeyValue("host.name", "record-1"))
	first, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	record.Attributes[0] = normalizetest.StringKeyValue("host.name", "record-2")
	second, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("different record attributes collapsed behind a resource attribute")
	}
}

func TestLogRejectsOverflowingObservedTimestamp(t *testing.T) {
	normalizer := New(normalizetest.Config())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}}
	record := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano()), ObservedTimeUnixNano: maxSignedNanoseconds + 1}
	if _, err := normalizer.logRecord(resource, "", nil, "", record); err == nil {
		t.Fatal("overflowing observed timestamp accepted")
	}
}

func TestLogRejectsInvalidSeverityNumber(t *testing.T) {
	normalizer := New(normalizetest.Config())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}}
	for _, number := range []logspb.SeverityNumber{-1, 25} {
		record := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano()), SeverityNumber: number}
		if _, err := normalizer.logRecord(resource, "", nil, "", record); err == nil {
			t.Fatalf("invalid severity %d accepted", number)
		}
	}
}

func TestSensitiveAttributeKeys(t *testing.T) {
	configuration := normalizetest.Config()
	configuration.RedactedKeys = []string{"internal.value"}
	normalizer := New(configuration)
	for _, key := range []string{"Authorization", "http.request.header.AUTHORIZATION", "Set-Cookie", "api/key", "credit.card", "user-password", "session_token", "internal.value"} {
		if !normalizer.redacts(key) {
			t.Fatalf("sensitive key %q was not redacted", key)
		}
	}
	for _, key := range []string{"service.name", "host.name", "http.response.status_code"} {
		if normalizer.redacts(key) {
			t.Fatalf("ordinary key %q was redacted", key)
		}
	}
}

func TestGenAIUsageCountersSurviveRedaction(t *testing.T) {
	integer := func(value int64) *commonpb.AnyValue {
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}
	}
	keyValue := func(key string, value *commonpb.AnyValue) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: key, Value: value}
	}
	attributes := []*commonpb.KeyValue{
		keyValue("gen_ai.usage.input_tokens", integer(0)),
		keyValue("gen_ai.usage.output_tokens", integer(42)),
		keyValue("gen_ai.usage.input_tokens_secret", integer(7)),
		keyValue("GEN_AI.USAGE.OUTPUT_TOKENS", integer(7)),
		keyValue("gen_ai.request.token", integer(7)),
		keyValue("nested", &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{
			Values: []*commonpb.KeyValue{keyValue("gen_ai.usage.input_tokens", integer(7))},
		}}}),
	}
	values, err := New(normalizetest.Config()).attributeValues(attributes)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"gen_ai.usage.input_tokens":        int64(0),
		"gen_ai.usage.output_tokens":       int64(42),
		"gen_ai.usage.input_tokens_secret": "[REDACTED]",
		"GEN_AI.USAGE.OUTPUT_TOKENS":       "[REDACTED]",
		"gen_ai.request.token":             "[REDACTED]",
		"nested":                           map[string]any{"gen_ai.usage.input_tokens": "[REDACTED]"},
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("attributes %#v, want %#v", values, want)
	}

	for name, value := range map[string]*commonpb.AnyValue{
		"string":   {Value: &commonpb.AnyValue_StringValue{StringValue: "12"}},
		"negative": integer(-1),
		"double":   {Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 12}},
		"empty":    {},
	} {
		values, err := New(normalizetest.Config()).attributeValues([]*commonpb.KeyValue{keyValue("gen_ai.usage.input_tokens", value)})
		if err != nil {
			t.Fatal(err)
		}
		if values["gen_ai.usage.input_tokens"] != "[REDACTED]" {
			t.Fatalf("%s counter was kept: %#v", name, values)
		}
	}

	values, err = New(normalizetest.Config()).attributeValues([]*commonpb.KeyValue{keyValue("service.name", &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api"}})})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := values["gen_ai.usage.input_tokens"]; found {
		t.Fatalf("absent counter appeared: %#v", values)
	}

	configuration := normalizetest.Config()
	configuration.RedactedKeys = []string{"Gen_AI.Usage.Input_Tokens"}
	values, err = New(configuration).attributeValues([]*commonpb.KeyValue{keyValue("gen_ai.usage.input_tokens", integer(5))})
	if err != nil {
		t.Fatal(err)
	}
	if values["gen_ai.usage.input_tokens"] != "[REDACTED]" {
		t.Fatalf("configured redaction was bypassed: %#v", values)
	}
}

func BenchmarkSensitiveAttributeKey(b *testing.B) {
	normalizer := New(normalizetest.Config())
	for b.Loop() {
		if !normalizer.redacts("http.request.header.authorization") || normalizer.redacts("service.name") {
			b.Fatal("redaction mismatch")
		}
	}
}

func TestLogsWithNULTextAreRejectedIndividually(t *testing.T) {
	normalizer := New(normalizetest.Config())
	stringBody := func(text string) *commonpb.AnyValue {
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: text}}
	}
	at := uint64(time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC).UnixNano())
	records := []*logspb.LogRecord{
		{TimeUnixNano: at, Body: stringBody("clean")},
		{TimeUnixNano: at + 1, Body: stringBody("bad\x00body")},
		{TimeUnixNano: at + 2, Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("note", "bad\x00value")}},
		{TimeUnixNano: at + 3, Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("bad\x00key", "value")}},
		{TimeUnixNano: at + 4, SeverityText: "ERR\x00R"},
	}
	events, rejected := normalizer.Logs([]*logspb.ResourceLogs{{
		Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}},
	}})
	if len(events) != 1 || rejected != 4 {
		t.Fatalf("kept %d and rejected %d records, want 1 and 4", len(events), rejected)
	}
}
