package otlp

import (
	"bytes"
	"strings"
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestScrubTextMasksCredentialShapes(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"key=value", "login failed password=hunter2 for bob", "login failed password=" + redacted + " for bob"},
		{"key: value", "api_key: sk_live_abc123 rejected", "api_key: " + redacted + " rejected"},
		{"prefixed name", "db_password=s3cret&host=db1", "db_password=" + redacted + "&host=db1"},
		{"quoted json", `{"user":"bob","token":"abcdef123456"}`, `{"user":"bob","token":"` + redacted + `"}`},
		{"authorization header", "Authorization: Bearer abc.def.ghi-123456", "Authorization: " + redacted},
		{"bare bearer", "sent Bearer abcdefgh12345678 upstream", "sent Bearer " + redacted + " upstream"},
		{"url query", "GET /reset?token=abc123&user=7", "GET /reset?token=" + redacted + "&user=7"},
		{"url userinfo", "connect postgres://app:p4ss@db.internal:5432/x failed", "connect postgres://" + redacted + "@db.internal:5432/x failed"},
		{"jwt", "bad jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk here", "bad jwt " + redacted + " here"},
		{"aws key id", "using AKIAIOSFODNN7EXAMPLE for s3", "using " + redacted + " for s3"},
		{"github token", "pushed with ghp_" + strings.Repeat("a", 36), "pushed with " + redacted},
		{"tracepilot key", "key tracepilot_" + strings.Repeat("Ab1_", 11) + " leaked", "key " + redacted + " leaked"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJB\n-----END RSA PRIVATE KEY----- tail", redacted + " tail"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := scrubText(test.input); got != test.want {
				t.Fatalf("scrubText(%q)\n got %q\nwant %q", test.input, got, test.want)
			}
		})
	}
}

func TestScrubTextLeavesOrdinaryTextAlone(t *testing.T) {
	for _, input := range []string{
		"user 42 not found",
		"GET /api/v1/orders/123 200",
		"max_tokens=100 tokenizer=fast",
		"the password is wrong", // prose without ':' or '=' has no recognisable shape
		"order total=19.99 currency=EUR",
		"https://example.com/docs?page=2",
		"short",
	} {
		if got := scrubText(input); got != input {
			t.Errorf("scrubText(%q) changed ordinary text to %q", input, got)
		}
	}
}

func TestScrubTextIsIdempotent(t *testing.T) {
	once := scrubText(`password=hunter2 Authorization: Bearer abcdefgh12345678 https://u:p@h/`)
	if twice := scrubText(once); twice != once {
		t.Fatalf("scrubbing twice changed the result:\n%q\n%q", once, twice)
	}
}

func TestNormalizerScrubsLogBodyExceptionAndStatusMessage(t *testing.T) {
	configuration := testIngestionConfig()
	configuration.ScrubFreeText = true
	normalizer := newNormalizer(configuration)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	events, rejected := normalizer.logs([]*logspb.ResourceLogs{{Resource: serviceResource(), ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{
		TimeUnixNano: uint64(now.UnixNano()), SeverityText: "ERROR",
		Body:       &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "login failed password=hunter2"}},
		Attributes: []*commonpb.KeyValue{stringKeyValue("exception.message", "bad token=abcdef123456")},
	}}}}}})
	if rejected != 0 || len(events) != 1 {
		t.Fatalf("rejected=%d events=%d", rejected, len(events))
	}
	event := events[0]
	for name, value := range map[string]string{"error message": event.ErrorMessage, "log body": toText(event.LogBody)} {
		if strings.Contains(value, "hunter2") || strings.Contains(value, "abcdef123456") || !strings.Contains(value, redacted) {
			t.Errorf("%s was not scrubbed: %q", name, value)
		}
	}

	span := &tracepb.Span{
		TraceId: bytes.Repeat([]byte{1}, 16), SpanId: bytes.Repeat([]byte{2}, 8), Name: "GET /reset?token=abc123",
		StartTimeUnixNano: uint64(now.UnixNano()), EndTimeUnixNano: uint64(now.Add(time.Millisecond).UnixNano()),
		Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "denied api_key=sk_live_abc"},
	}
	traceEvents, traceRejected := normalizer.traces([]*tracepb.ResourceSpans{{Resource: serviceResource(), ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}}}})
	if traceRejected != 0 || len(traceEvents) != 1 {
		t.Fatalf("rejected=%d events=%d", traceRejected, len(traceEvents))
	}
	traceEvent := traceEvents[0]
	for name, value := range map[string]string{"operation": traceEvent.Operation, "status message": traceEvent.StatusMessage, "error message": traceEvent.ErrorMessage} {
		if strings.Contains(value, "abc123") || strings.Contains(value, "sk_live_abc") {
			t.Errorf("%s was not scrubbed: %q", name, value)
		}
	}
}

func TestNormalizerKeepsFreeTextWhenScrubbingIsOff(t *testing.T) {
	configuration := testIngestionConfig()
	configuration.ScrubFreeText = false
	normalizer := newNormalizer(configuration)
	if got := normalizer.scrub("password=hunter2"); got != "password=hunter2" {
		t.Fatalf("scrubbing ran while disabled: %q", got)
	}
}

func BenchmarkScrubTextOrdinary(b *testing.B) {
	for range b.N {
		scrubText("GET /api/v1/orders/123 completed in 14ms for customer 42")
	}
}

func BenchmarkScrubTextTriggered(b *testing.B) {
	for range b.N {
		scrubText("login failed password=hunter2 for bob from 10.0.0.1 Authorization: Bearer abcdefgh12345678")
	}
}

func serviceResource() *resourcepb.Resource {
	return &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "checkout")}}
}

func toText(value any) string {
	text, _ := value.(string)
	return text
}
