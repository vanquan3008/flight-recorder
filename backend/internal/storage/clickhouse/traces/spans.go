package traces

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

const SpanColumns = `event_id, trace_id, span_id, parent_span_id, trace_state, span_name, span_kind,
started_at, ended_at, duration_ns, status_code, status_message, flags, service_name, service_version,
resource_schema_url, resource_attributes, scope_name, scope_version, scope_schema_url, scope_attributes,
span_attributes, span_events, span_links, dropped_attributes_count, dropped_events_count, dropped_links_count,
endpoint, request_failed, dependency_type, dependency_name`

// storedSpan derives endpoint, request_failed and the dependency columns at
// insert, so reads aggregate them without parsing attributes.
type storedSpan struct {
	ProjectID              string            `json:"project_id"`
	EnvironmentID          string            `json:"environment_id"`
	EventID                string            `json:"event_id"`
	TraceID                string            `json:"trace_id"`
	SpanID                 string            `json:"span_id"`
	ParentSpanID           string            `json:"parent_span_id"`
	TraceState             string            `json:"trace_state"`
	SpanName               string            `json:"span_name"`
	SpanKind               int32             `json:"span_kind"`
	StartedAt              time.Time         `json:"started_at"`
	EndedAt                time.Time         `json:"ended_at"`
	DurationNanos          int64             `json:"duration_ns"`
	StatusCode             string            `json:"status_code"`
	StatusMessage          string            `json:"status_message"`
	Flags                  uint32            `json:"flags"`
	ServiceName            string            `json:"service_name"`
	ServiceVersion         string            `json:"service_version"`
	ResourceSchemaURL      string            `json:"resource_schema_url"`
	ResourceAttributes     map[string]string `json:"resource_attributes"`
	ScopeName              string            `json:"scope_name"`
	ScopeVersion           string            `json:"scope_version"`
	ScopeSchemaURL         string            `json:"scope_schema_url"`
	ScopeAttributes        map[string]string `json:"scope_attributes"`
	SpanAttributes         map[string]string `json:"span_attributes"`
	SpanEvents             string            `json:"span_events"`
	SpanLinks              string            `json:"span_links"`
	DroppedAttributesCount uint32            `json:"dropped_attributes_count"`
	DroppedEventsCount     uint32            `json:"dropped_events_count"`
	DroppedLinksCount      uint32            `json:"dropped_links_count"`
	Endpoint               string            `json:"endpoint"`
	RequestFailed          bool              `json:"request_failed"`
	DependencyType         string            `json:"dependency_type"`
	DependencyName         string            `json:"dependency_name"`
}

func (store *Traces) StoreSpans(ctx context.Context, projectID, environmentID string, events []telemetry.Event) error {
	if err := client.ValidateScope(projectID, environmentID); err != nil {
		return err
	}
	rows := make([]storedSpan, 0, len(events))
	for _, event := range events {
		if event.ProjectID != projectID || event.EnvironmentID != environmentID || event.Signal != telemetry.SignalTrace {
			return fmt.Errorf("ClickHouse span ingestion scope mismatch: %w", telemetry.ErrInvalidEvent)
		}
		row, err := eventSpan(event)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return client.Insert(ctx, store.client, "tracepilot_spans", rows)
}

// eventSpan keeps the span events and links in the JSON shape the API returns.
func eventSpan(event telemetry.Event) (storedSpan, error) {
	if !telemetry.ValidUUID(event.ID) || event.Timestamp.IsZero() || event.TraceID == "" || event.SpanID == "" || event.Duration < 0 {
		return storedSpan{}, fmt.Errorf("invalid span identity or timestamp: %w", telemetry.ErrInvalidEvent)
	}
	dependencyType, dependencyName := telemetry.SpanDependency(event.RecordAttributes, event.SpanKind)
	row := storedSpan{
		ProjectID: event.ProjectID, EnvironmentID: event.EnvironmentID, EventID: event.ID,
		TraceID: event.TraceID, SpanID: event.SpanID, ParentSpanID: event.ParentSpanID, TraceState: event.TraceState,
		SpanName: event.Operation, SpanKind: event.SpanKind,
		StartedAt: event.Timestamp.UTC(), EndedAt: event.Timestamp.Add(event.Duration).UTC(), DurationNanos: event.Duration.Nanoseconds(),
		StatusCode: event.Status, StatusMessage: event.StatusMessage, Flags: event.TraceFlags,
		ServiceName: event.ServiceName, ServiceVersion: event.ServiceVersion,
		ResourceSchemaURL: event.ResourceSchemaURL, ScopeSchemaURL: event.ScopeSchemaURL,
		ScopeName: event.ScopeName, ScopeVersion: event.ScopeVersion,
		DroppedAttributesCount: event.DroppedAttributes, DroppedEventsCount: event.DroppedEvents, DroppedLinksCount: event.DroppedLinks,
		Endpoint: telemetry.SpanEndpoint(event.RecordAttributes, event.Operation), RequestFailed: telemetry.SpanRequestFailed(event.Status, event.RecordAttributes),
		DependencyType: dependencyType, DependencyName: dependencyName,
	}
	var err error
	if row.ResourceAttributes, err = client.AttributeMap(event.ResourceAttributes); err != nil {
		return row, err
	}
	if row.ScopeAttributes, err = client.AttributeMap(event.ScopeAttributes); err != nil {
		return row, err
	}
	if row.SpanAttributes, err = client.AttributeMap(event.RecordAttributes); err != nil {
		return row, err
	}
	if row.SpanEvents, err = client.JSONArray(event.SpanEvents); err != nil {
		return row, err
	}
	row.SpanLinks, err = client.JSONArray(event.SpanLinks)
	return row, err
}
