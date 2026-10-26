package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func queueSignalRecord(batch *pgx.Batch, ingestion config.IngestionConfig, event telemetry.Event) (int, error) {
	switch event.Signal {
	case "":
		return 0, nil
	case telemetry.SignalTrace:
		resourceAttributes, resourceError := marshalJSONObject(event.ResourceAttributes)
		if resourceError != nil {
			return 0, fmt.Errorf("encode span resource attributes: %w", resourceError)
		}
		scopeAttributes, scopeError := marshalJSONObject(event.ScopeAttributes)
		if scopeError != nil {
			return 0, fmt.Errorf("encode span scope attributes: %w", scopeError)
		}
		recordAttributes, recordError := marshalJSONObject(event.RecordAttributes)
		if recordError != nil {
			return 0, fmt.Errorf("encode span attributes: %w", recordError)
		}
		spanEvents, eventsError := marshalJSONArray(event.SpanEvents)
		if eventsError != nil {
			return 0, fmt.Errorf("encode span events: %w", eventsError)
		}
		spanLinks, linksError := marshalJSONArray(event.SpanLinks)
		if linksError != nil {
			return 0, fmt.Errorf("encode span links: %w", linksError)
		}
		batch.Queue(`
			INSERT INTO spans (
				event_id, project_id, environment_id, trace_id, span_id, parent_span_id,
				trace_state, span_name, span_kind, started_at, ended_at, status_code, status_message, flags,
				service_name, service_version, resource_schema_url, resource_attributes,
				scope_name, scope_version, scope_schema_url, scope_attributes, span_attributes,
				span_events, span_links, dropped_attributes_count, dropped_events_count, dropped_links_count
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, $8, $9, $10, $11, $12, $13, $14,
				$15, $16, $17, $18,
				$19, $20, $21, $22, $23,
				$24, $25, $26, $27, $28
			)
			ON CONFLICT (event_id, started_at) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.TraceID, event.SpanID, nullableString(event.ParentSpanID),
			event.TraceState, event.Operation, event.SpanKind, event.Timestamp, event.Timestamp.Add(event.Duration), event.Status, event.StatusMessage, int64(event.TraceFlags),
			event.ServiceName, nullableString(event.ServiceVersion), event.ResourceSchemaURL, resourceAttributes,
			event.ScopeName, event.ScopeVersion, event.ScopeSchemaURL, scopeAttributes, recordAttributes,
			spanEvents, spanLinks, event.DroppedAttributes, event.DroppedEvents, event.DroppedLinks,
		)
		return 1, nil
	case telemetry.SignalLog:
		resourceAttributes, resourceError := marshalJSONObject(event.ResourceAttributes)
		if resourceError != nil {
			return 0, fmt.Errorf("encode log resource attributes: %w", resourceError)
		}
		scopeAttributes, scopeError := marshalJSONObject(event.ScopeAttributes)
		if scopeError != nil {
			return 0, fmt.Errorf("encode log scope attributes: %w", scopeError)
		}
		recordAttributes, recordError := marshalJSONObject(event.RecordAttributes)
		if recordError != nil {
			return 0, fmt.Errorf("encode log attributes: %w", recordError)
		}
		body, bodyError := marshalJSON(event.LogBody)
		if bodyError != nil {
			return 0, fmt.Errorf("encode log body: %w", bodyError)
		}
		batch.Queue(`
			INSERT INTO logs (
				event_id, project_id, environment_id, occurred_at, observed_at, trace_id, span_id,
				trace_flags, severity_number, severity_text, body, event_name, service_name, service_version,
				resource_schema_url, resource_attributes, scope_name, scope_version, scope_schema_url,
				scope_attributes, log_attributes, dropped_attributes_count
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, $11, $12, $13, $14,
				$15, $16, $17, $18, $19,
				$20, $21, $22
			)
			ON CONFLICT (event_id, occurred_at) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.Timestamp, nullableTime(event.ObservedTimestamp), nullableString(event.TraceID), nullableString(event.SpanID),
			int64(event.TraceFlags), event.SeverityNumber, event.Severity, body, event.LogEventName, event.ServiceName, nullableString(event.ServiceVersion),
			event.ResourceSchemaURL, resourceAttributes, event.ScopeName, event.ScopeVersion, event.ScopeSchemaURL,
			scopeAttributes, recordAttributes, event.DroppedAttributes,
		)
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported telemetry signal %q", event.Signal)
	}
}
