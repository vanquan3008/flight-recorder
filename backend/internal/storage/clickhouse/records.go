package clickhouse

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

const logColumns = `event_id, occurred_at, observed_at, trace_id, span_id, trace_flags,
severity_number, severity_text, body, event_name, service_name, service_version,
resource_attributes, scope_name, scope_version, scope_attributes, attributes`

type storedLog struct {
	ProjectID          string     `json:"project_id"`
	EnvironmentID      string     `json:"environment_id"`
	EventID            string     `json:"event_id"`
	OccurredAt         time.Time  `json:"occurred_at"`
	ObservedAt         *time.Time `json:"observed_at"`
	TraceID            string     `json:"trace_id"`
	SpanID             string     `json:"span_id"`
	TraceFlags         int64      `json:"trace_flags"`
	SeverityNumber     int32      `json:"severity_number"`
	SeverityText       string     `json:"severity_text"`
	Body               string     `json:"body"`
	EventName          string     `json:"event_name"`
	ServiceName        string     `json:"service_name"`
	ServiceVersion     string     `json:"service_version"`
	ResourceAttributes string     `json:"resource_attributes"`
	ScopeName          string     `json:"scope_name"`
	ScopeVersion       string     `json:"scope_version"`
	ScopeAttributes    string     `json:"scope_attributes"`
	Attributes         string     `json:"attributes"`
}

func (row storedLog) record() (query.LogRecord, error) {
	for _, raw := range []string{row.Body, row.ResourceAttributes, row.ScopeAttributes, row.Attributes} {
		if !json.Valid([]byte(raw)) {
			return query.LogRecord{}, fmt.Errorf("invalid JSON in stored log")
		}
	}
	return query.LogRecord{
		EventID: row.EventID, OccurredAt: row.OccurredAt, ObservedAt: row.ObservedAt,
		TraceID: row.TraceID, SpanID: row.SpanID, TraceFlags: row.TraceFlags,
		SeverityNumber: row.SeverityNumber, SeverityText: row.SeverityText, Body: json.RawMessage(row.Body),
		EventName: row.EventName, ServiceName: row.ServiceName, ServiceVersion: row.ServiceVersion,
		ResourceAttributes: json.RawMessage(row.ResourceAttributes), ScopeName: row.ScopeName, ScopeVersion: row.ScopeVersion,
		ScopeAttributes: json.RawMessage(row.ScopeAttributes), Attributes: json.RawMessage(row.Attributes),
	}, nil
}

func jsonText(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode log value: %w", telemetry.ErrInvalidEvent)
	}
	return string(data), nil
}

func jsonObject(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	return jsonText(value)
}

func eventLog(event telemetry.Event) (storedLog, error) {
	row := storedLog{
		ProjectID: event.ProjectID, EnvironmentID: event.EnvironmentID, EventID: event.ID,
		OccurredAt: event.Timestamp.UTC(), TraceID: event.TraceID, SpanID: event.SpanID, TraceFlags: int64(event.TraceFlags),
		SeverityNumber: event.SeverityNumber, SeverityText: event.Severity, EventName: event.LogEventName,
		ServiceName: event.ServiceName, ServiceVersion: event.ServiceVersion, ScopeName: event.ScopeName, ScopeVersion: event.ScopeVersion,
	}
	if !event.ObservedTimestamp.IsZero() {
		observed := event.ObservedTimestamp.UTC()
		row.ObservedAt = &observed
	}
	var err error
	if row.Body, err = jsonText(event.LogBody); err != nil {
		return row, err
	}
	if row.ResourceAttributes, err = jsonObject(event.ResourceAttributes); err != nil {
		return row, err
	}
	if row.ScopeAttributes, err = jsonObject(event.ScopeAttributes); err != nil {
		return row, err
	}
	row.Attributes, err = jsonObject(event.RecordAttributes)
	return row, err
}
