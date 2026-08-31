package otlp

import (
	"encoding/hex"
	"errors"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func (normalizer *normalizer) span(resource *resourcepb.Resource, resourceSchemaURL string, scope *commonpb.InstrumentationScope, scopeSchemaURL string, span *tracepb.Span) (telemetry.Event, error) {
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
		if addError := attributes.addAll(spanEvent.GetAttributes()); addError != nil {
			return telemetry.Event{}, addError
		}
	}
	for _, link := range span.GetLinks() {
		if addError := attributes.addAll(link.GetAttributes()); addError != nil {
			return telemetry.Event{}, addError
		}
	}
	if textError := normalizer.validateText(resourceSchemaURL, scope.GetName(), scope.GetVersion(), scopeSchemaURL, span.GetName(), span.GetStatus().GetMessage(), span.GetTraceState()); textError != nil {
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
	resourceAttributes, resourceAttributesError := normalizer.attributeValues(resource.GetAttributes())
	if resourceAttributesError != nil {
		return telemetry.Event{}, resourceAttributesError
	}
	scopeAttributes, scopeAttributesError := normalizer.attributeValues(scope.GetAttributes())
	if scopeAttributesError != nil {
		return telemetry.Event{}, scopeAttributesError
	}
	recordAttributes, recordAttributesError := normalizer.attributeValues(span.GetAttributes())
	if recordAttributesError != nil {
		return telemetry.Event{}, recordAttributesError
	}
	spanEvents, spanEventsError := normalizer.normalizeSpanEvents(span.GetEvents())
	if spanEventsError != nil {
		return telemetry.Event{}, spanEventsError
	}
	spanLinks, spanLinksError := normalizer.normalizeSpanLinks(span.GetLinks())
	if spanLinksError != nil {
		return telemetry.Event{}, spanLinksError
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
		ProjectID:          normalizer.configuration.ProjectID,
		Environment:        normalizer.configuration.EnvironmentName,
		ServiceName:        stringAttribute(attributes.values, "service.name"),
		ServiceVersion:     stringAttribute(attributes.values, "service.version"),
		TraceID:            traceID,
		SpanID:             spanID,
		ParentSpanID:       hex.EncodeToString(span.GetParentSpanId()),
		RequestID:          requestID(attributes.values),
		Type:               eventType,
		Timestamp:          timestamp,
		Duration:           endTimestamp.Sub(timestamp),
		Status:             spanStatus(span),
		HTTPStatus:         httpStatus,
		Operation:          span.GetName(),
		ErrorType:          errorType,
		ErrorMessage:       errorMessage,
		Attributes:         attributes.values,
		Signal:             telemetry.SignalTrace,
		ResourceAttributes: resourceAttributes,
		ResourceSchemaURL:  resourceSchemaURL,
		ScopeName:          scope.GetName(),
		ScopeVersion:       scope.GetVersion(),
		ScopeSchemaURL:     scopeSchemaURL,
		ScopeAttributes:    scopeAttributes,
		RecordAttributes:   recordAttributes,
		TraceState:         span.GetTraceState(),
		TraceFlags:         span.GetFlags(),
		SpanKind:           int32(span.GetKind()),
		StatusMessage:      span.GetStatus().GetMessage(),
		DroppedAttributes:  span.GetDroppedAttributesCount(),
		DroppedEvents:      span.GetDroppedEventsCount(),
		DroppedLinks:       span.GetDroppedLinksCount(),
		SpanEvents:         spanEvents,
		SpanLinks:          spanLinks,
	}
	event.ID = telemetry.DeterministicID(timestamp, normalizer.configuration.ProjectID, traceID, spanID)
	if validationError := event.Validate(); validationError != nil {
		return telemetry.Event{}, validationError
	}
	return event, nil
}
