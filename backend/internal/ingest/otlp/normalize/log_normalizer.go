package normalize

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// logRecord falls back to the observed time; a body over the value limit
// rejects the record.
func (normalizer *Normalizer) logRecord(resource *resourcepb.Resource, resourceSchemaURL string, scope *commonpb.InstrumentationScope, scopeSchemaURL string, record *logspb.LogRecord) (telemetry.Event, error) {
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
	origin, originError := normalizer.source(resource, resourceSchemaURL, scope, scopeSchemaURL, record.GetAttributes())
	if originError != nil {
		return telemetry.Event{}, originError
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
		TraceID:           traceID,
		SpanID:            spanID,
		Type:              eventType,
		Timestamp:         timestamp,
		Severity:          severity,
		Operation:         scope.GetName(),
		ErrorType:         errorType,
		ErrorMessage:      errorMessage,
		Signal:            telemetry.SignalLog,
		TraceFlags:        record.GetFlags(),
		ObservedTimestamp: unixNanos(record.GetObservedTimeUnixNano()),
		SeverityNumber:    int32(record.GetSeverityNumber()),
		DroppedAttributes: record.GetDroppedAttributesCount(),
		LogBody:           body,
		LogEventName:      record.GetEventName(),
	}
	normalizer.stamp(&event, origin, attributes.values)
	identityAttributes, _ := json.Marshal([]any{origin.resourceAttributes, origin.scopeAttributes, origin.recordAttributes, body})
	event.ID = telemetry.DeterministicID(
		timestamp,
		timestamp.Format(time.RFC3339Nano),
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
