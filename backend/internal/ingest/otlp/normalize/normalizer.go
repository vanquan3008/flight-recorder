package normalize

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const maxSignedNanoseconds = uint64(1<<63 - 1)

// Normalizer turns OTLP export requests into TracePilot telemetry for one project and environment.
type Normalizer struct {
	configuration config.IngestionConfig
	redactedKeys  map[string]struct{}
}

// New builds a normalizer that stamps every record with the configured scope and redacts the
// configured keys.
func New(configuration config.IngestionConfig) *Normalizer {
	redactedKeys := make(map[string]struct{}, len(configuration.RedactedKeys))
	for _, key := range configuration.RedactedKeys {
		redactedKeys[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	return &Normalizer{configuration: configuration, redactedKeys: redactedKeys}
}

// Traces normalizes every span of the request and counts the ones it rejected.
func (normalizer *Normalizer) Traces(resourceSpans []*tracepb.ResourceSpans) ([]telemetry.Event, int) {
	events := make([]telemetry.Event, 0)
	rejected := 0
	sequence := 0
	for _, resourceGroup := range resourceSpans {
		for _, scopeGroup := range resourceGroup.GetScopeSpans() {
			for _, span := range scopeGroup.GetSpans() {
				sequence++
				if sequence > normalizer.configuration.MaxRecordsPerRequest {
					rejected++
					continue
				}
				event, normalizeError := normalizer.span(resourceGroup.GetResource(), resourceGroup.GetSchemaUrl(), scopeGroup.GetScope(), scopeGroup.GetSchemaUrl(), span)
				if normalizeError != nil {
					rejected++
					continue
				}
				events = append(events, event)
			}
		}
	}
	return events, rejected
}

// Logs normalizes every log record of the request and counts the ones it rejected.
func (normalizer *Normalizer) Logs(resourceLogs []*logspb.ResourceLogs) ([]telemetry.Event, int) {
	events := make([]telemetry.Event, 0)
	rejected := 0
	sequence := 0
	for _, resourceGroup := range resourceLogs {
		for _, scopeGroup := range resourceGroup.GetScopeLogs() {
			for _, record := range scopeGroup.GetLogRecords() {
				sequence++
				if sequence > normalizer.configuration.MaxRecordsPerRequest {
					rejected++
					continue
				}
				event, normalizeError := normalizer.logRecord(resourceGroup.GetResource(), resourceGroup.GetSchemaUrl(), scopeGroup.GetScope(), scopeGroup.GetSchemaUrl(), record)
				if normalizeError != nil {
					rejected++
					continue
				}
				events = append(events, event)
			}
		}
	}
	return events, rejected
}

func (normalizer *Normalizer) attributeValues(attributes []*commonpb.KeyValue) (map[string]any, error) {
	builder := newAttributeBuilder(normalizer)
	if addError := builder.addAll(attributes); addError != nil {
		return nil, addError
	}
	return builder.values, nil
}

func (normalizer *Normalizer) normalizeSpanEvents(events []*tracepb.Span_Event) ([]telemetry.SpanEvent, error) {
	result := make([]telemetry.SpanEvent, 0, len(events))
	for _, event := range events {
		if event.GetTimeUnixNano() > maxSignedNanoseconds {
			return nil, errors.New("span event timestamp is invalid")
		}
		if textError := normalizer.validateText(event.GetName()); textError != nil {
			return nil, textError
		}
		attributes, attributesError := normalizer.attributeValues(event.GetAttributes())
		if attributesError != nil {
			return nil, attributesError
		}
		result = append(result, telemetry.SpanEvent{
			Timestamp:         unixNanos(event.GetTimeUnixNano()),
			Name:              event.GetName(),
			Attributes:        attributes,
			DroppedAttributes: event.GetDroppedAttributesCount(),
		})
	}
	return result, nil
}

func (normalizer *Normalizer) normalizeSpanLinks(links []*tracepb.Span_Link) ([]telemetry.SpanLink, error) {
	result := make([]telemetry.SpanLink, 0, len(links))
	for _, link := range links {
		if !validIdentifier(link.GetTraceId(), 16, false) || !validIdentifier(link.GetSpanId(), 8, false) {
			return nil, errors.New("span link identity is invalid")
		}
		if textError := normalizer.validateText(link.GetTraceState()); textError != nil {
			return nil, textError
		}
		attributes, attributesError := normalizer.attributeValues(link.GetAttributes())
		if attributesError != nil {
			return nil, attributesError
		}
		result = append(result, telemetry.SpanLink{
			TraceID:           hex.EncodeToString(link.GetTraceId()),
			SpanID:            hex.EncodeToString(link.GetSpanId()),
			TraceState:        link.GetTraceState(),
			Flags:             link.GetFlags(),
			Attributes:        attributes,
			DroppedAttributes: link.GetDroppedAttributesCount(),
		})
	}
	return result, nil
}

func unixNanos(value uint64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(value)).UTC()
}

// usageCounters are OpenTelemetry GenAI counters whose names contain "token"
// but whose values are counts, not credentials.
var usageCounters = map[string]struct{}{
	"gen_ai.usage.input_tokens":  {},
	"gen_ai.usage.output_tokens": {},
}

// usageCounter keeps a GenAI usage counter that the built-in "token" pattern
// would otherwise mask. Only an exact top-level key with a nonnegative integer
// qualifies, and a configured redaction still wins. Anything else falls through
// to redacts and is masked.
func (normalizer *Normalizer) usageCounter(key string, value *commonpb.AnyValue) (int64, bool) {
	if _, counter := usageCounters[key]; !counter {
		return 0, false
	}
	if _, configured := normalizer.redactedKeys[strings.ToLower(key)]; configured {
		return 0, false
	}
	integer, isInteger := value.GetValue().(*commonpb.AnyValue_IntValue)
	if !isInteger || integer.IntValue < 0 {
		return 0, false
	}
	return integer.IntValue, true
}

// redacts matches built-in patterns anywhere in the key. GenAI usage counters
// are exempted earlier by usageCounter.
func (normalizer *Normalizer) redacts(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if _, found := normalizer.redactedKeys[normalized]; found {
		return true
	}
	for _, sensitivePattern := range []string{
		"authorization", "cookie", "password", "passwd", "secret", "token", "apikey",
		"api_key", "api.key", "api-key", "api/key",
		"credit_card", "credit.card", "credit-card", "credit/card",
	} {
		if strings.Contains(normalized, sensitivePattern) {
			return true
		}
	}
	return false
}

func (normalizer *Normalizer) validateText(values ...string) error {
	for _, value := range values {
		if len(value) > normalizer.configuration.MaxAttributeValueBytes {
			return errors.New("record text exceeds limit")
		}
		if containsNUL(value) {
			return errNULText
		}
	}
	return nil
}

// errNULText rejects one record up front: PostgreSQL text and jsonb cannot hold
// U+0000, and a stored batch containing it would fail for every record in it.
var errNULText = errors.New("record text contains a NUL character")

func containsNUL(value string) bool {
	return strings.IndexByte(value, 0) >= 0
}

// spanEventType maps SERVER and CLIENT spans to the HTTP types even when they
// are not HTTP.
func spanEventType(span *tracepb.Span, attributes map[string]any) telemetry.EventType {
	if _, found := attributes["exception.type"]; found {
		return telemetry.EventException
	}
	if _, found := attributes["exception.message"]; found {
		return telemetry.EventException
	}
	if _, found := attributes["db.system"]; found {
		return telemetry.EventDatabase
	}
	if _, found := attributes["rpc.system"]; found {
		return telemetry.EventRPC
	}
	if _, found := attributes["messaging.system"]; found {
		return telemetry.EventMessage
	}
	switch span.GetKind() {
	case tracepb.Span_SPAN_KIND_SERVER:
		return telemetry.EventHTTPServer
	case tracepb.Span_SPAN_KIND_CLIENT:
		return telemetry.EventHTTPClient
	default:
		return telemetry.EventCustom
	}
}

func spanStatus(span *tracepb.Span) string {
	return strings.TrimPrefix(span.GetStatus().GetCode().String(), "STATUS_CODE_")
}

func requestID(attributes map[string]any) string {
	return stringAttribute(attributes, "http.request.header.x-request-id", "http.request.header.x_request_id", "request.id")
}

func stringAttribute(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, found := attributes[key]; found {
			if text, valid := value.(string); valid {
				return text
			}
		}
	}
	return ""
}

// intAttribute rejects the record when the first present key is not an int64
// HTTP status.
func intAttribute(attributes map[string]any, keys ...string) (int, error) {
	for _, key := range keys {
		value, found := attributes[key]
		if !found {
			continue
		}
		integer, valid := value.(int64)
		if !valid || integer < 100 || integer > 599 {
			return 0, errors.New("HTTP status attribute is invalid")
		}
		return int(integer), nil
	}
	return 0, nil
}

// validIdentifier rejects all-zero IDs, as W3C Trace Context requires.
func validIdentifier(value []byte, size int, optional bool) bool {
	if len(value) == 0 {
		return optional
	}
	if len(value) != size {
		return false
	}
	for _, part := range value {
		if part != 0 {
			return true
		}
	}
	return false
}

func isErrorSeverity(values ...string) bool {
	for _, value := range values {
		normalized := strings.ToUpper(value)
		if strings.Contains(normalized, "ERROR") || strings.Contains(normalized, "FATAL") {
			return true
		}
	}
	return false
}
