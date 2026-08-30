package otlp

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var defaultRedactedSegments = map[string]struct{}{
	"authorization": {}, "cookie": {}, "password": {}, "passwd": {},
	"secret": {}, "token": {}, "api_key": {}, "apikey": {},
	"credit_card": {}, "set_cookie": {},
}

const maxSignedNanoseconds = uint64(1<<63 - 1)

type normalizer struct {
	configuration config.IngestionConfig
	redactedKeys  map[string]struct{}
}

func newNormalizer(configuration config.IngestionConfig) *normalizer {
	redactedKeys := make(map[string]struct{}, len(configuration.RedactedKeys))
	for _, key := range configuration.RedactedKeys {
		redactedKeys[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	return &normalizer{configuration: configuration, redactedKeys: redactedKeys}
}

func (normalizer *normalizer) traces(resourceSpans []*tracepb.ResourceSpans) ([]telemetry.Event, int) {
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

func (normalizer *normalizer) logs(resourceLogs []*logspb.ResourceLogs) ([]telemetry.Event, int) {
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

func (normalizer *normalizer) attributeValues(attributes []*commonpb.KeyValue) (map[string]any, error) {
	builder := newAttributeBuilder(normalizer)
	if addError := builder.addAll(attributes); addError != nil {
		return nil, addError
	}
	return builder.values, nil
}

func (normalizer *normalizer) normalizeSpanEvents(events []*tracepb.Span_Event) ([]telemetry.SpanEvent, error) {
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

func (normalizer *normalizer) normalizeSpanLinks(links []*tracepb.Span_Link) ([]telemetry.SpanLink, error) {
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

func (normalizer *normalizer) redacts(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if _, found := normalizer.redactedKeys[normalized]; found {
		return true
	}
	canonical := strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(normalized)
	if _, found := defaultRedactedSegments[canonical]; found {
		return true
	}
	for _, sensitivePattern := range []string{"authorization", "cookie", "password", "passwd", "secret", "token", "api_key", "apikey", "credit_card"} {
		if strings.Contains(canonical, sensitivePattern) {
			return true
		}
	}
	segments := strings.FieldsFunc(normalized, func(character rune) bool {
		return character == '.' || character == '-' || character == '/' || character == '_'
	})
	for _, segment := range segments {
		if _, found := defaultRedactedSegments[segment]; found {
			return true
		}
	}
	return false
}

func (normalizer *normalizer) validateText(values ...string) error {
	for _, value := range values {
		if len(value) > normalizer.configuration.MaxAttributeValueBytes {
			return errors.New("record text exceeds limit")
		}
	}
	return nil
}

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
