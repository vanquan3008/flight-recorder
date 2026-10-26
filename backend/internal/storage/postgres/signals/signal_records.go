package signals

import (
	"context"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// queueSignalRecord queues the span or log row that keeps the full OTLP shape
// of an event. Events without a signal (detector output) have none.
func queueSignalRecord(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, event telemetry.Event) error {
	switch event.Signal {
	case "":
		return nil
	case telemetry.SignalTrace:
		return queueSpan(ctx, writes, ingestion, event)
	case telemetry.SignalLog:
		return queueLog(ctx, writes, ingestion, event)
	default:
		return fmt.Errorf("unsupported telemetry signal %q", event.Signal)
	}
}

func queueSpan(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, event telemetry.Event) error {
	resourceAttributes, resourceError := pgvalue.MarshalJSONObject(event.ResourceAttributes)
	if resourceError != nil {
		return fmt.Errorf("encode span resource attributes: %w", resourceError)
	}
	scopeAttributes, scopeError := pgvalue.MarshalJSONObject(event.ScopeAttributes)
	if scopeError != nil {
		return fmt.Errorf("encode span scope attributes: %w", scopeError)
	}
	spanAttributes, recordError := pgvalue.MarshalJSONObject(event.RecordAttributes)
	if recordError != nil {
		return fmt.Errorf("encode span attributes: %w", recordError)
	}
	spanEvents, eventsError := pgvalue.MarshalJSONArray(event.SpanEvents)
	if eventsError != nil {
		return fmt.Errorf("encode span events: %w", eventsError)
	}
	spanLinks, linksError := pgvalue.MarshalJSONArray(event.SpanLinks)
	if linksError != nil {
		return fmt.Errorf("encode span links: %w", linksError)
	}
	writes.InsertSpan(ctx, sqlcstore.InsertSpanParams{
		EventID: event.ID, ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		TraceID: event.TraceID, SpanID: event.SpanID, ParentSpanID: pgvalue.OrNil(event.ParentSpanID),
		TraceState: event.TraceState, SpanName: event.Operation, SpanKind: event.SpanKind,
		StartedAt: event.Timestamp, EndedAt: event.Timestamp.Add(event.Duration),
		StatusCode: event.Status, StatusMessage: event.StatusMessage, Flags: int64(event.TraceFlags),
		ServiceName: event.ServiceName, ServiceVersion: pgvalue.OrNil(event.ServiceVersion),
		ResourceSchemaURL: event.ResourceSchemaURL, ResourceAttributes: resourceAttributes,
		ScopeName: event.ScopeName, ScopeVersion: event.ScopeVersion, ScopeSchemaURL: event.ScopeSchemaURL,
		ScopeAttributes: scopeAttributes, SpanAttributes: spanAttributes,
		SpanEvents: spanEvents, SpanLinks: spanLinks,
		DroppedAttributesCount: pgvalue.ClampInt32(event.DroppedAttributes),
		DroppedEventsCount:     pgvalue.ClampInt32(event.DroppedEvents),
		DroppedLinksCount:      pgvalue.ClampInt32(event.DroppedLinks),
	})
	return nil
}

func queueLog(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, event telemetry.Event) error {
	resourceAttributes, resourceError := pgvalue.MarshalJSONObject(event.ResourceAttributes)
	if resourceError != nil {
		return fmt.Errorf("encode log resource attributes: %w", resourceError)
	}
	scopeAttributes, scopeError := pgvalue.MarshalJSONObject(event.ScopeAttributes)
	if scopeError != nil {
		return fmt.Errorf("encode log scope attributes: %w", scopeError)
	}
	logAttributes, recordError := pgvalue.MarshalJSONObject(event.RecordAttributes)
	if recordError != nil {
		return fmt.Errorf("encode log attributes: %w", recordError)
	}
	body, bodyError := pgvalue.MarshalJSON(event.LogBody)
	if bodyError != nil {
		return fmt.Errorf("encode log body: %w", bodyError)
	}
	writes.InsertLog(ctx, sqlcstore.InsertLogParams{
		EventID: event.ID, ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		OccurredAt: event.Timestamp, ObservedAt: pgvalue.TimeOrNil(event.ObservedTimestamp),
		TraceID: pgvalue.OrNil(event.TraceID), SpanID: pgvalue.OrNil(event.SpanID),
		TraceFlags: int64(event.TraceFlags), SeverityNumber: event.SeverityNumber, SeverityText: event.Severity,
		Body: body, EventName: event.LogEventName,
		ServiceName: event.ServiceName, ServiceVersion: pgvalue.OrNil(event.ServiceVersion),
		ResourceSchemaURL: event.ResourceSchemaURL, ResourceAttributes: resourceAttributes,
		ScopeName: event.ScopeName, ScopeVersion: event.ScopeVersion, ScopeSchemaURL: event.ScopeSchemaURL,
		ScopeAttributes: scopeAttributes, LogAttributes: logAttributes,
		DroppedAttributesCount: pgvalue.ClampInt32(event.DroppedAttributes),
	})
	return nil
}
