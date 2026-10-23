package otlp

import (
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestLogIDsIncludeEnvironment(t *testing.T) {
	configuration := testIngestionConfig()
	first := newNormalizer(configuration)
	configuration.EnvironmentID = "01900000-0000-7000-8000-000000000003"
	second := newNormalizer(configuration)
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker")}}
	const timestamp = uint64(1791507600123456789)
	record := &logspb.LogRecord{TimeUnixNano: timestamp}
	firstLog, err := first.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	secondLog, err := second.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	if firstLog.ID == secondLog.ID {
		t.Fatal("logs in different environments received the same primary key")
	}
}

func TestLogIdentityKeepsAttributeOrigins(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker"), stringKeyValue("host.name", "host-1")}}
	record := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano())}
	record.Attributes = append(record.Attributes, stringKeyValue("host.name", "record-1"))
	first, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	record.Attributes[0] = stringKeyValue("host.name", "record-2")
	second, err := normalizer.logRecord(resource, "", nil, "", record)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("different record attributes collapsed behind a resource attribute")
	}
}

func TestLogRejectsOverflowingObservedTimestamp(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker")}}
	record := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano()), ObservedTimeUnixNano: maxSignedNanoseconds + 1}
	if _, err := normalizer.logRecord(resource, "", nil, "", record); err == nil {
		t.Fatal("overflowing observed timestamp accepted")
	}
}

func TestLogRejectsInvalidSeverityNumber(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker")}}
	for _, number := range []logspb.SeverityNumber{-1, 25} {
		record := &logspb.LogRecord{TimeUnixNano: uint64(time.Now().UnixNano()), SeverityNumber: number}
		if _, err := normalizer.logRecord(resource, "", nil, "", record); err == nil {
			t.Fatalf("invalid severity %d accepted", number)
		}
	}
}

func TestSensitiveAttributeKeys(t *testing.T) {
	configuration := testIngestionConfig()
	configuration.RedactedKeys = []string{"internal.value"}
	normalizer := newNormalizer(configuration)
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

func BenchmarkSensitiveAttributeKey(b *testing.B) {
	normalizer := newNormalizer(testIngestionConfig())
	for b.Loop() {
		if !normalizer.redacts("http.request.header.authorization") || normalizer.redacts("service.name") {
			b.Fatal("redaction mismatch")
		}
	}
}
