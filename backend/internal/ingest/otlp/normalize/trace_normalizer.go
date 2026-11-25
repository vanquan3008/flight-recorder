package normalize

import (
	"encoding/hex"
	"errors"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// span merges resource, scope, span, event and link attributes into one map.
// Limits apply to that map, and resource keys win on collision.
func (normalizer *Normalizer) span(resource *resourcepb.Resource, resourceSchemaURL string, scope *commonpb.InstrumentationScope, scopeSchemaURL string, span *tracepb.Span) (telemetry.Event, error) {
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
	origin, originError := normalizer.source(resource, resourceSchemaURL, scope, scopeSchemaURL, span.GetAttributes())
	if originError != nil {
		return telemetry.Event{}, originError
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
	statusMessage := normalizer.scrub(span.GetStatus().GetMessage())
	operation := normalizer.scrub(span.GetName())
	errorType := stringAttribute(attributes.values, "exception.type")
	errorMessage := stringAttribute(attributes.values, "exception.message")
	if errorMessage == "" && span.GetStatus().GetCode() == tracepb.Status_STATUS_CODE_ERROR {
		errorMessage = statusMessage
	}
	httpStatus, httpStatusError := intAttribute(attributes.values, "http.response.status_code", "http.status_code")
	if httpStatusError != nil {
		return telemetry.Event{}, httpStatusError
	}
	event := telemetry.Event{
		TraceID:           traceID,
		SpanID:            spanID,
		ParentSpanID:      hex.EncodeToString(span.GetParentSpanId()),
		Type:              eventType,
		Timestamp:         timestamp,
		Duration:          endTimestamp.Sub(timestamp),
		Status:            spanStatus(span),
		HTTPStatus:        httpStatus,
		Operation:         operation,
		ErrorType:         errorType,
		ErrorMessage:      errorMessage,
		Signal:            telemetry.SignalTrace,
		TraceState:        span.GetTraceState(),
		TraceFlags:        span.GetFlags(),
		SpanKind:          int32(span.GetKind()),
		StatusMessage:     statusMessage,
		DroppedAttributes: span.GetDroppedAttributesCount(),
		DroppedEvents:     span.GetDroppedEventsCount(),
		DroppedLinks:      span.GetDroppedLinksCount(),
		SpanEvents:        spanEvents,
		SpanLinks:         spanLinks,
	}
	normalizer.stamp(&event, origin, attributes.values)
	event.ID = telemetry.DeterministicID(timestamp, normalizer.configuration.ProjectID, normalizer.configuration.EnvironmentID, traceID, spanID)
	if validationError := event.Validate(); validationError != nil {
		return telemetry.Event{}, validationError
	}
	return event, nil
}
