package otlp

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func (normalizer *normalizer) logRecord(resource *resourcepb.Resource, resourceSchemaURL string, scope *commonpb.InstrumentationScope, scopeSchemaURL string, record *logspb.LogRecord) (telemetry.Event, error) {
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
	if textError := normalizer.validateText(resourceSchemaURL, scope.GetName(), scope.GetVersion(), scopeSchemaURL, record.GetSeverityText(), record.GetEventName()); textError != nil {
		return telemetry.Event{}, textError
	}
	resourceAttributes, resourceAttributesError := normalizer.attributeValues(resource.GetAttributes())
	if resourceAttributesError != nil {
		return telemetry.Event{}, resourceAttributesError
	}
	scopeAttributes, scopeAttributesError := normalizer.attributeValues(scope.GetAttributes())
	if scopeAttributesError != nil {
		return telemetry.Event{}, scopeAttributesError
	}
	recordAttributes, recordAttributesError := normalizer.attributeValues(record.GetAttributes())
	if recordAttributesError != nil {
		return telemetry.Event{}, recordAttributesError
	}
	bodyText := fmt.Sprint(body)
	if addError := attributes.addSynthetic("log.body", body); addError != nil {
		return telemetry.Event{}, addError
	}
	timestampNanos := record.GetTimeUnixNano()
	if timestampNanos == 0 {
		timestampNanos = record.GetObservedTimeUnixNano()
	}
	if timestampNanos == 0 || timestampNanos > maxSignedNanoseconds || record.GetObservedTimeUnixNano() > maxSignedNanoseconds || record.GetSeverityNumber() < 0 || record.GetSeverityNumber() > logspb.SeverityNumber_SEVERITY_NUMBER_FATAL4 || !validIdentifier(record.GetTraceId(), 16, true) || !validIdentifier(record.GetSpanId(), 8, true) {
		return telemetry.Event{}, errors.New("log identity, severity or timestamp is invalid")
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
		ProjectID:          normalizer.configuration.ProjectID,
		EnvironmentID:      normalizer.configuration.EnvironmentID,
		Environment:        normalizer.configuration.EnvironmentName,
		ServiceName:        stringAttribute(attributes.values, "service.name"),
		ServiceVersion:     stringAttribute(attributes.values, "service.version"),
		TraceID:            traceID,
		SpanID:             spanID,
		RequestID:          requestID(attributes.values),
		Type:               eventType,
		Timestamp:          timestamp,
		Severity:           severity,
		Operation:          scope.GetName(),
		ErrorType:          errorType,
		ErrorMessage:       errorMessage,
		Attributes:         attributes.values,
		Signal:             telemetry.SignalLog,
		ResourceAttributes: resourceAttributes,
		ResourceSchemaURL:  resourceSchemaURL,
		ScopeName:          scope.GetName(),
		ScopeVersion:       scope.GetVersion(),
		ScopeSchemaURL:     scopeSchemaURL,
		ScopeAttributes:    scopeAttributes,
		RecordAttributes:   recordAttributes,
		TraceFlags:         record.GetFlags(),
		ObservedTimestamp:  unixNanos(record.GetObservedTimeUnixNano()),
		SeverityNumber:     int32(record.GetSeverityNumber()),
		DroppedAttributes:  record.GetDroppedAttributesCount(),
		LogBody:            body,
		LogEventName:       record.GetEventName(),
	}
	identityAttributes, _ := json.Marshal([]any{resourceAttributes, scopeAttributes, recordAttributes, body})
	event.ID = telemetry.DeterministicID(
		timestamp,
		normalizer.configuration.ProjectID,
		normalizer.configuration.EnvironmentID,
		event.ServiceName,
		resourceSchemaURL,
		scope.GetName(),
		scope.GetVersion(),
		scopeSchemaURL,
		traceID,
		spanID,
		record.GetSeverityNumber().String(),
		record.GetSeverityText(),
		record.GetEventName(),
		fmt.Sprintf("%d", record.GetObservedTimeUnixNano()),
		fmt.Sprintf("%d", record.GetFlags()),
		fmt.Sprintf("%d", record.GetDroppedAttributesCount()),
		string(identityAttributes),
	)
	if validationError := event.Validate(); validationError != nil {
		return telemetry.Event{}, validationError
	}
	return event, nil
}
