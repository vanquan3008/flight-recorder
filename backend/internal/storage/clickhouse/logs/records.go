package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

const Columns = `event_id, occurred_at, observed_at, trace_id, span_id, trace_flags,
severity_number, severity_text, body, body_is_json, event_name, service_name, service_version,
resource_schema_url, resource_attributes, scope_name, scope_version, scope_schema_url, scope_attributes,
attributes, dropped_attributes_count`

type storedLog struct {
	ProjectID              string            `json:"project_id"`
	EnvironmentID          string            `json:"environment_id"`
	EventID                string            `json:"event_id"`
	OccurredAt             time.Time         `json:"occurred_at"`
	ObservedAt             *time.Time        `json:"observed_at"`
	TraceID                string            `json:"trace_id"`
	SpanID                 string            `json:"span_id"`
	TraceFlags             uint32            `json:"trace_flags"`
	SeverityNumber         uint8             `json:"severity_number"`
	SeverityText           string            `json:"severity_text"`
	Body                   string            `json:"body"`
	BodyIsJSON             bool              `json:"body_is_json"`
	EventName              string            `json:"event_name"`
	ServiceName            string            `json:"service_name"`
	ServiceVersion         string            `json:"service_version"`
	ResourceSchemaURL      string            `json:"resource_schema_url"`
	ResourceAttributes     map[string]string `json:"resource_attributes"`
	ScopeName              string            `json:"scope_name"`
	ScopeVersion           string            `json:"scope_version"`
	ScopeSchemaURL         string            `json:"scope_schema_url"`
	ScopeAttributes        map[string]string `json:"scope_attributes"`
	Attributes             map[string]string `json:"attributes"`
	DroppedAttributesCount uint32            `json:"dropped_attributes_count"`
}

func (row storedLog) record() (query.LogRecord, error) {
	body := json.RawMessage(row.Body)
	if !row.BodyIsJSON {
		body, _ = json.Marshal(row.Body) // a string always encodes
	} else if !json.Valid(body) {
		return query.LogRecord{}, errors.New("invalid JSON in stored log")
	}
	return query.LogRecord{
		EventID: row.EventID, OccurredAt: row.OccurredAt, ObservedAt: row.ObservedAt,
		TraceID: row.TraceID, SpanID: row.SpanID, TraceFlags: int64(row.TraceFlags),
		SeverityNumber: int32(row.SeverityNumber), SeverityText: row.SeverityText, Body: body,
		EventName: row.EventName, ServiceName: row.ServiceName, ServiceVersion: row.ServiceVersion,
		ResourceSchemaURL: row.ResourceSchemaURL, ResourceAttributes: client.AttributeJSON(row.ResourceAttributes),
		ScopeName: row.ScopeName, ScopeVersion: row.ScopeVersion, ScopeSchemaURL: row.ScopeSchemaURL,
		ScopeAttributes: client.AttributeJSON(row.ScopeAttributes), Attributes: client.AttributeJSON(row.Attributes),
		DroppedAttributesCount: row.DroppedAttributesCount,
	}, nil
}

// RecordOf is the record a stored log event reads back as, which lets a test prove storage keeps
// every field of the log.
func RecordOf(event telemetry.Event) (query.LogRecord, error) {
	row, err := eventLog(event)
	if err != nil {
		return query.LogRecord{}, err
	}
	return row.record()
}

func eventLog(event telemetry.Event) (storedLog, error) {
	if event.SeverityNumber < 0 || event.SeverityNumber > math.MaxUint8 {
		return storedLog{}, fmt.Errorf("invalid log severity: %w", telemetry.ErrInvalidEvent)
	}
	row := storedLog{
		ProjectID: event.ProjectID, EnvironmentID: event.EnvironmentID, EventID: event.ID,
		OccurredAt: event.Timestamp.UTC(), TraceID: event.TraceID, SpanID: event.SpanID, TraceFlags: event.TraceFlags,
		SeverityNumber: uint8(event.SeverityNumber), SeverityText: event.Severity, EventName: event.LogEventName,
		ServiceName: event.ServiceName, ServiceVersion: event.ServiceVersion, ScopeName: event.ScopeName, ScopeVersion: event.ScopeVersion,
		ResourceSchemaURL: event.ResourceSchemaURL, ScopeSchemaURL: event.ScopeSchemaURL, DroppedAttributesCount: event.DroppedAttributes,
	}
	if !event.ObservedTimestamp.IsZero() {
		observed := event.ObservedTimestamp.UTC()
		row.ObservedAt = &observed
	}
	var err error
	if text, isText := event.LogBody.(string); isText {
		row.Body = text
	} else {
		if row.Body, err = client.JSONText(event.LogBody); err != nil {
			return row, err
		}
		row.BodyIsJSON = true
	}
	if row.ResourceAttributes, err = client.AttributeMap(event.ResourceAttributes); err != nil {
		return row, err
	}
	if row.ScopeAttributes, err = client.AttributeMap(event.ScopeAttributes); err != nil {
		return row, err
	}
	row.Attributes, err = client.AttributeMap(event.RecordAttributes)
	return row, err
}
