package otlp

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/flight-observability/flight/internal/config"
	"github.com/flight-observability/flight/internal/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
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
				event, normalizeError := normalizer.span(resourceGroup.GetResource(), scopeGroup.GetScope(), span)
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
				event, normalizeError := normalizer.logRecord(resourceGroup.GetResource(), scopeGroup.GetScope(), record)
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

func (normalizer *normalizer) span(resource *resourcepb.Resource, scope *commonpb.InstrumentationScope, span *tracepb.Span) (telemetry.Event, error) {
	attributes := newAttributeBuilder(normalizer)
	if addError := attributes.addAll(resource.GetAttributes()); addError != nil {
		return telemetry.Event{}, addError
	}
	if addError := attributes.addAll(scope.GetAttributes()); addError != nil {
		return telemetry.Event{}, addError
	}
	if addError := attributes.addAll(span.GetAttributes()); addError != nil {
		return telemetry.Event{}, addError
	}
	for _, spanEvent := range span.GetEvents() {
		if spanEvent.GetName() == "exception" {
			if addError := attributes.addAll(spanEvent.GetAttributes()); addError != nil {
				return telemetry.Event{}, addError
			}
		}
	}
	if textError := normalizer.validateText(scope.GetName(), scope.GetVersion(), span.GetName(), span.GetStatus().GetMessage()); textError != nil {
		return telemetry.Event{}, textError
	}
	if scope.GetName() != "" {
		if addError := attributes.addSynthetic("otel.scope.name", scope.GetName()); addError != nil {
			return telemetry.Event{}, addError
		}
	}
	if scope.GetVersion() != "" {
		if addError := attributes.addSynthetic("otel.scope.version", scope.GetVersion()); addError != nil {
			return telemetry.Event{}, addError
		}
	}

	if span.GetStartTimeUnixNano() == 0 || span.GetStartTimeUnixNano() > maxSignedNanoseconds || span.GetEndTimeUnixNano() > maxSignedNanoseconds || span.GetEndTimeUnixNano() < span.GetStartTimeUnixNano() || span.GetEndTimeUnixNano()-span.GetStartTimeUnixNano() > maxSignedNanoseconds || !validIdentifier(span.GetTraceId(), 16, false) || !validIdentifier(span.GetSpanId(), 8, false) || !validIdentifier(span.GetParentSpanId(), 8, true) {
		return telemetry.Event{}, errors.New("span identity or timestamps are invalid")
	}
	timestamp := time.Unix(0, int64(span.GetStartTimeUnixNano())).UTC()
	endTimestamp := time.Unix(0, int64(span.GetEndTimeUnixNano())).UTC()
	eventType := spanEventType(span, attributes.values)
	traceID := hex.EncodeToString(span.GetTraceId())
	spanID := hex.EncodeToString(span.GetSpanId())
	errorType := stringAttribute(attributes.values, "exception.type")
	errorMessage := stringAttribute(attributes.values, "exception.message")
	if errorMessage == "" && span.GetStatus().GetCode() == tracepb.Status_STATUS_CODE_ERROR {
		errorMessage = span.GetStatus().GetMessage()
	}
	httpStatus, httpStatusError := intAttribute(attributes.values, "http.response.status_code", "http.status_code")
	if httpStatusError != nil {
		return telemetry.Event{}, httpStatusError
	}
	event := telemetry.Event{
		ProjectID:      normalizer.configuration.ProjectID,
		Environment:    normalizer.configuration.EnvironmentName,
		ServiceName:    stringAttribute(attributes.values, "service.name"),
		ServiceVersion: stringAttribute(attributes.values, "service.version"),
		TraceID:        traceID,
		SpanID:         spanID,
		ParentSpanID:   hex.EncodeToString(span.GetParentSpanId()),
		RequestID:      requestID(attributes.values),
		Type:           eventType,
		Timestamp:      timestamp,
		Duration:       endTimestamp.Sub(timestamp),
		Status:         spanStatus(span),
		HTTPStatus:     httpStatus,
		Operation:      span.GetName(),
		ErrorType:      errorType,
		ErrorMessage:   errorMessage,
		Attributes:     attributes.values,
	}
	event.ID = telemetry.DeterministicID(timestamp, normalizer.configuration.ProjectID, traceID, spanID)
	if validationError := event.Validate(); validationError != nil {
		return telemetry.Event{}, validationError
	}
	return event, nil
}

func (normalizer *normalizer) logRecord(resource *resourcepb.Resource, scope *commonpb.InstrumentationScope, record *logspb.LogRecord) (telemetry.Event, error) {
	attributes := newAttributeBuilder(normalizer)
	if addError := attributes.addAll(resource.GetAttributes()); addError != nil {
		return telemetry.Event{}, addError
	}
	if addError := attributes.addAll(scope.GetAttributes()); addError != nil {
		return telemetry.Event{}, addError
	}
	if addError := attributes.addAll(record.GetAttributes()); addError != nil {
		return telemetry.Event{}, addError
	}
	body, bodyError := attributes.value(record.GetBody(), 0)
	if bodyError != nil {
		return telemetry.Event{}, bodyError
	}
	bodyJSON, marshalError := json.Marshal(body)
	if marshalError != nil || len(bodyJSON) > normalizer.configuration.MaxAttributeValueBytes {
		return telemetry.Event{}, errors.New("log body is invalid")
	}
	if textError := normalizer.validateText(scope.GetName(), scope.GetVersion(), record.GetSeverityText()); textError != nil {
		return telemetry.Event{}, textError
	}
	bodyText := fmt.Sprint(body)
	if addError := attributes.addSynthetic("log.body", body); addError != nil {
		return telemetry.Event{}, addError
	}
	timestampNanos := record.GetTimeUnixNano()
	if timestampNanos == 0 {
		timestampNanos = record.GetObservedTimeUnixNano()
	}
	if timestampNanos == 0 || timestampNanos > maxSignedNanoseconds || !validIdentifier(record.GetTraceId(), 16, true) || !validIdentifier(record.GetSpanId(), 8, true) {
		return telemetry.Event{}, errors.New("log identity or timestamp is invalid")
	}
	timestamp := time.Unix(0, int64(timestampNanos)).UTC()
	traceID := hex.EncodeToString(record.GetTraceId())
	spanID := hex.EncodeToString(record.GetSpanId())
	errorType := stringAttribute(attributes.values, "exception.type")
	errorMessage := stringAttribute(attributes.values, "exception.message")
	eventType := telemetry.EventLog
	if errorType != "" || errorMessage != "" {
		eventType = telemetry.EventException
	}
	if errorMessage == "" && isErrorSeverity(record.GetSeverityText(), record.GetSeverityNumber().String()) {
		errorMessage = bodyText
	}
	severity := strings.TrimSpace(record.GetSeverityText())
	if severity == "" {
		severity = strings.TrimPrefix(record.GetSeverityNumber().String(), "SEVERITY_NUMBER_")
	}
	event := telemetry.Event{
		ProjectID:      normalizer.configuration.ProjectID,
		Environment:    normalizer.configuration.EnvironmentName,
		ServiceName:    stringAttribute(attributes.values, "service.name"),
		ServiceVersion: stringAttribute(attributes.values, "service.version"),
		TraceID:        traceID,
		SpanID:         spanID,
		RequestID:      requestID(attributes.values),
		Type:           eventType,
		Timestamp:      timestamp,
		Severity:       severity,
		Operation:      scope.GetName(),
		ErrorType:      errorType,
		ErrorMessage:   errorMessage,
		Attributes:     attributes.values,
	}
	identityAttributes, _ := json.Marshal(attributes.values)
	// ponytail: OTLP logs have no universal source ID, so exact duplicates at
	// one timestamp collapse; include a source ID here if the protocol adds one.
	event.ID = telemetry.DeterministicID(timestamp, normalizer.configuration.ProjectID, event.ServiceName, scope.GetName(), scope.GetVersion(), traceID, spanID, record.GetSeverityNumber().String(), record.GetSeverityText(), string(identityAttributes))
	if validationError := event.Validate(); validationError != nil {
		return telemetry.Event{}, validationError
	}
	return event, nil
}

type attributeBuilder struct {
	normalizer *normalizer
	values     map[string]any
	count      int
}

func newAttributeBuilder(normalizer *normalizer) *attributeBuilder {
	return &attributeBuilder{normalizer: normalizer, values: make(map[string]any)}
}

func (builder *attributeBuilder) addAll(attributes []*commonpb.KeyValue) error {
	for _, attribute := range attributes {
		key := strings.TrimSpace(attribute.GetKey())
		if addError := builder.add(key, attribute.GetValue()); addError != nil {
			return addError
		}
	}
	return nil
}

func (builder *attributeBuilder) add(key string, value *commonpb.AnyValue) error {
	if builder.normalizer.redacts(key) {
		return builder.addSynthetic(key, "[REDACTED]")
	}
	converted, convertError := builder.value(value, 0)
	if convertError != nil {
		return convertError
	}
	return builder.addSynthetic(key, converted)
}

func (builder *attributeBuilder) addSynthetic(key string, value any) error {
	builder.count++
	if builder.count > builder.normalizer.configuration.MaxAttributesPerRecord {
		return errors.New("attribute count exceeds limit")
	}
	if key == "" || len(key) > builder.normalizer.configuration.MaxAttributeValueBytes {
		return errors.New("attribute key is empty or too long")
	}
	if builder.normalizer.redacts(key) {
		value = "[REDACTED]"
	}
	encodedValue, encodeError := json.Marshal(value)
	if encodeError != nil || len(encodedValue) > builder.normalizer.configuration.MaxAttributeValueBytes {
		return errors.New("encoded attribute value exceeds limit")
	}
	if _, exists := builder.values[key]; !exists {
		builder.values[key] = value
	}
	return nil
}

func (builder *attributeBuilder) value(value *commonpb.AnyValue, depth int) (any, error) {
	if value == nil {
		return nil, nil
	}
	if depth > builder.normalizer.configuration.MaxAttributeDepth {
		return nil, errors.New("attribute nesting exceeds limit")
	}
	switch typedValue := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		if len(typedValue.StringValue) > builder.normalizer.configuration.MaxAttributeValueBytes {
			return nil, errors.New("attribute string exceeds limit")
		}
		return typedValue.StringValue, nil
	case *commonpb.AnyValue_BoolValue:
		return typedValue.BoolValue, nil
	case *commonpb.AnyValue_IntValue:
		return typedValue.IntValue, nil
	case *commonpb.AnyValue_DoubleValue:
		if math.IsNaN(typedValue.DoubleValue) || math.IsInf(typedValue.DoubleValue, 0) {
			return nil, errors.New("attribute number is not finite")
		}
		return typedValue.DoubleValue, nil
	case *commonpb.AnyValue_BytesValue:
		if len(typedValue.BytesValue) > builder.normalizer.configuration.MaxAttributeValueBytes {
			return nil, errors.New("attribute bytes exceed limit")
		}
		return base64.StdEncoding.EncodeToString(typedValue.BytesValue), nil
	case *commonpb.AnyValue_ArrayValue:
		values := typedValue.ArrayValue.GetValues()
		if len(values) > builder.normalizer.configuration.MaxAttributesPerRecord {
			return nil, errors.New("attribute array exceeds limit")
		}
		result := make([]any, 0, len(values))
		for _, nested := range values {
			converted, convertError := builder.value(nested, depth+1)
			if convertError != nil {
				return nil, convertError
			}
			result = append(result, converted)
		}
		return result, nil
	case *commonpb.AnyValue_KvlistValue:
		result := make(map[string]any)
		for _, nested := range typedValue.KvlistValue.GetValues() {
			builder.count++
			if builder.count > builder.normalizer.configuration.MaxAttributesPerRecord {
				return nil, errors.New("attribute count exceeds limit")
			}
			key := strings.TrimSpace(nested.GetKey())
			if key == "" || len(key) > builder.normalizer.configuration.MaxAttributeValueBytes {
				return nil, errors.New("attribute key is empty or too long")
			}
			if builder.normalizer.redacts(key) {
				result[key] = "[REDACTED]"
				continue
			}
			converted, convertError := builder.value(nested.GetValue(), depth+1)
			if convertError != nil {
				return nil, convertError
			}
			if _, exists := result[key]; !exists {
				result[key] = converted
			}
		}
		return result, nil
	default:
		return nil, nil
	}
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
