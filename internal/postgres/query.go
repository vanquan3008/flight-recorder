package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type TraceFilter struct {
	Service   string
	Operation string
	Status    string
	TraceID   string
	From      time.Time
	To        time.Time
	Before    time.Time
	CursorID  string
	Limit     int
}

type TraceSummary struct {
	TraceID       string    `json:"trace_id"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	CursorAt      time.Time `json:"-"`
	DurationNanos int64     `json:"duration_nanos"`
	RootService   string    `json:"root_service"`
	SpanCount     int       `json:"span_count"`
	ErrorCount    int       `json:"error_count"`
}

type SpanRecord struct {
	EventID            string          `json:"event_id"`
	TraceID            string          `json:"trace_id"`
	SpanID             string          `json:"span_id"`
	ParentSpanID       string          `json:"parent_span_id,omitempty"`
	TraceState         string          `json:"trace_state,omitempty"`
	Name               string          `json:"name"`
	Kind               int32           `json:"kind"`
	StartedAt          time.Time       `json:"started_at"`
	EndedAt            time.Time       `json:"ended_at"`
	StatusCode         string          `json:"status_code"`
	StatusMessage      string          `json:"status_message,omitempty"`
	Flags              int64           `json:"flags"`
	ServiceName        string          `json:"service_name"`
	ServiceVersion     string          `json:"service_version,omitempty"`
	ResourceAttributes json.RawMessage `json:"resource_attributes"`
	ScopeName          string          `json:"scope_name,omitempty"`
	ScopeVersion       string          `json:"scope_version,omitempty"`
	ScopeAttributes    json.RawMessage `json:"scope_attributes"`
	Attributes         json.RawMessage `json:"attributes"`
	Events             json.RawMessage `json:"events"`
	Links              json.RawMessage `json:"links"`
}

type LogFilter struct {
	EventID  string
	Service  string
	Severity string
	TraceID  string
	SpanID   string
	Search   string
	From     time.Time
	To       time.Time
	Before   time.Time
	CursorID string
	Limit    int
}

type LogRecord struct {
	EventID            string          `json:"event_id"`
	OccurredAt         time.Time       `json:"occurred_at"`
	ObservedAt         *time.Time      `json:"observed_at,omitempty"`
	TraceID            string          `json:"trace_id,omitempty"`
	SpanID             string          `json:"span_id,omitempty"`
	TraceFlags         int64           `json:"trace_flags"`
	SeverityNumber     int32           `json:"severity_number"`
	SeverityText       string          `json:"severity_text,omitempty"`
	Body               json.RawMessage `json:"body"`
	EventName          string          `json:"event_name,omitempty"`
	ServiceName        string          `json:"service_name"`
	ServiceVersion     string          `json:"service_version,omitempty"`
	ResourceAttributes json.RawMessage `json:"resource_attributes"`
	ScopeName          string          `json:"scope_name,omitempty"`
	ScopeVersion       string          `json:"scope_version,omitempty"`
	ScopeAttributes    json.RawMessage `json:"scope_attributes"`
	Attributes         json.RawMessage `json:"attributes"`
}

type ServiceSummary struct {
	Name           string  `json:"name"`
	TraceCount     int64   `json:"trace_count"`
	RequestCount   int64   `json:"request_count"`
	ErrorCount     int64   `json:"error_count"`
	ErrorRate      float64 `json:"error_rate"`
	AverageLatency int64   `json:"average_latency_nanos"`
}

type ServiceDependency struct {
	ServiceName string `json:"service_name"`
	CallCount   int64  `json:"call_count"`
}

type Incident struct {
	ID              string    `json:"id"`
	PrimaryService  string    `json:"primary_service"`
	TraceID         string    `json:"trace_id,omitempty"`
	RequestID       string    `json:"request_id,omitempty"`
	Fingerprint     string    `json:"fingerprint"`
	Title           string    `json:"title"`
	ErrorType       string    `json:"error_type,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	HTTPStatus      *int      `json:"http_status,omitempty"`
	Severity        string    `json:"severity"`
	Status          string    `json:"status"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	OccurrenceCount int64     `json:"occurrence_count"`
}

func (database *Database) ListTraces(ctx context.Context, projectID string, filter TraceFilter) ([]TraceSummary, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT
			trace_id,
			min(started_at),
			max(ended_at),
			max(started_at),
			(extract(epoch FROM max(ended_at) - min(started_at)) * 1000000000)::bigint,
			(array_agg(service_name ORDER BY started_at, event_id))[1],
			count(*),
			count(*) FILTER (WHERE status_code = 'ERROR')
		FROM spans
		WHERE project_id = $1
			AND ($2 = '' OR service_name = $2)
			AND ($3 = '' OR span_name = $3)
			AND ($4 = '' OR status_code = $4)
			AND ($5 = '' OR trace_id = $5)
			AND ($6::timestamptz IS NULL OR started_at >= $6)
			AND ($7::timestamptz IS NULL OR started_at <= $7)
		GROUP BY trace_id
		HAVING $8::timestamptz IS NULL
			OR max(started_at) < $8
			OR (max(started_at) = $8 AND trace_id < $9)
		ORDER BY max(started_at) DESC, trace_id DESC
		LIMIT $10`,
		projectID, filter.Service, filter.Operation, filter.Status, filter.TraceID,
		nullableTime(filter.From), nullableTime(filter.To), nullableTime(filter.Before), filter.CursorID, filter.Limit)
	if queryError != nil {
		return nil, fmt.Errorf("list traces: %w", queryError)
	}
	defer rows.Close()

	traces := make([]TraceSummary, 0, filter.Limit)
	for rows.Next() {
		var trace TraceSummary
		if scanError := rows.Scan(&trace.TraceID, &trace.StartedAt, &trace.EndedAt, &trace.CursorAt, &trace.DurationNanos, &trace.RootService, &trace.SpanCount, &trace.ErrorCount); scanError != nil {
			return nil, fmt.Errorf("scan trace: %w", scanError)
		}
		traces = append(traces, trace)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate traces: %w", rowsError)
	}
	return traces, nil
}

func (database *Database) GetTrace(ctx context.Context, projectID, traceID string) ([]SpanRecord, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT event_id, trace_id, span_id, COALESCE(parent_span_id, ''), trace_state,
			span_name, span_kind, started_at, ended_at, status_code, status_message, flags,
			service_name, COALESCE(service_version, ''), resource_attributes,
			scope_name, scope_version, scope_attributes, span_attributes, span_events, span_links
		FROM spans
		WHERE project_id = $1 AND trace_id = $2
		ORDER BY started_at, event_id`, projectID, traceID)
	if queryError != nil {
		return nil, fmt.Errorf("get trace: %w", queryError)
	}
	defer rows.Close()

	spans := make([]SpanRecord, 0)
	for rows.Next() {
		var span SpanRecord
		if scanError := rows.Scan(
			&span.EventID, &span.TraceID, &span.SpanID, &span.ParentSpanID, &span.TraceState,
			&span.Name, &span.Kind, &span.StartedAt, &span.EndedAt, &span.StatusCode, &span.StatusMessage, &span.Flags,
			&span.ServiceName, &span.ServiceVersion, &span.ResourceAttributes,
			&span.ScopeName, &span.ScopeVersion, &span.ScopeAttributes, &span.Attributes, &span.Events, &span.Links,
		); scanError != nil {
			return nil, fmt.Errorf("scan span: %w", scanError)
		}
		spans = append(spans, span)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate spans: %w", rowsError)
	}
	return spans, nil
}

func (database *Database) ListLogs(ctx context.Context, projectID string, filter LogFilter) ([]LogRecord, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT event_id, occurred_at, observed_at, COALESCE(trace_id, ''), COALESCE(span_id, ''),
			trace_flags, severity_number, severity_text, body, event_name, service_name,
			COALESCE(service_version, ''), resource_attributes, scope_name, scope_version,
			scope_attributes, log_attributes
		FROM logs
		WHERE project_id = $1
			AND ($2 = '' OR service_name = $2)
			AND ($3 = '' OR severity_text = $3)
			AND ($4 = '' OR trace_id = $4)
			AND ($5 = '' OR span_id = $5)
			AND ($6::uuid IS NULL OR event_id = $6::uuid)
			AND ($7 = '' OR body::text ILIKE '%' || $7 || '%')
			AND ($8::timestamptz IS NULL OR occurred_at >= $8)
			AND ($9::timestamptz IS NULL OR occurred_at <= $9)
			AND ($10::timestamptz IS NULL OR occurred_at < $10 OR (occurred_at = $10 AND event_id < $11::uuid))
		ORDER BY occurred_at DESC, event_id DESC
		LIMIT $12`,
		projectID, filter.Service, filter.Severity, filter.TraceID, filter.SpanID, nullableString(filter.EventID), filter.Search,
		nullableTime(filter.From), nullableTime(filter.To), nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit)
	if queryError != nil {
		return nil, fmt.Errorf("list logs: %w", queryError)
	}
	defer rows.Close()

	logs := make([]LogRecord, 0, filter.Limit)
	for rows.Next() {
		var record LogRecord
		if scanError := rows.Scan(
			&record.EventID, &record.OccurredAt, &record.ObservedAt, &record.TraceID, &record.SpanID,
			&record.TraceFlags, &record.SeverityNumber, &record.SeverityText, &record.Body, &record.EventName, &record.ServiceName,
			&record.ServiceVersion, &record.ResourceAttributes, &record.ScopeName, &record.ScopeVersion,
			&record.ScopeAttributes, &record.Attributes,
		); scanError != nil {
			return nil, fmt.Errorf("scan log: %w", scanError)
		}
		logs = append(logs, record)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate logs: %w", rowsError)
	}
	return logs, nil
}

func (database *Database) GetLog(ctx context.Context, projectID, eventID string) (LogRecord, error) {
	logs, queryError := database.ListLogs(ctx, projectID, LogFilter{EventID: eventID, Limit: 1})
	if queryError != nil {
		return LogRecord{}, queryError
	}
	for _, record := range logs {
		if record.EventID == eventID {
			return record, nil
		}
	}
	return LogRecord{}, nil
}

func (database *Database) ListServices(ctx context.Context, projectID string, from, to time.Time) ([]ServiceSummary, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT service_name,
			count(DISTINCT trace_id),
			count(*) FILTER (WHERE span_kind = 2),
			count(*) FILTER (WHERE status_code = 'ERROR'),
			COALESCE(count(*) FILTER (WHERE status_code = 'ERROR')::double precision / NULLIF(count(*), 0), 0),
			COALESCE(avg(extract(epoch FROM ended_at - started_at) * 1000000000)::bigint, 0)
		FROM spans
		WHERE project_id = $1
			AND ($2::timestamptz IS NULL OR started_at >= $2)
			AND ($3::timestamptz IS NULL OR started_at <= $3)
		GROUP BY service_name
		ORDER BY service_name`, projectID, nullableTime(from), nullableTime(to))
	if queryError != nil {
		return nil, fmt.Errorf("list services: %w", queryError)
	}
	defer rows.Close()
	services := make([]ServiceSummary, 0)
	for rows.Next() {
		var service ServiceSummary
		if scanError := rows.Scan(&service.Name, &service.TraceCount, &service.RequestCount, &service.ErrorCount, &service.ErrorRate, &service.AverageLatency); scanError != nil {
			return nil, fmt.Errorf("scan service: %w", scanError)
		}
		services = append(services, service)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate services: %w", rowsError)
	}
	return services, nil
}

func (database *Database) ListServiceDependencies(ctx context.Context, projectID, serviceName string) ([]ServiceDependency, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT child.service_name, count(*)
		FROM spans child
		JOIN spans parent ON parent.project_id = child.project_id
			AND parent.trace_id = child.trace_id
			AND parent.span_id = child.parent_span_id
		WHERE child.project_id = $1 AND parent.service_name = $2 AND parent.service_name <> child.service_name
		GROUP BY child.service_name
		ORDER BY count(*) DESC, child.service_name`, projectID, serviceName)
	if queryError != nil {
		return nil, fmt.Errorf("list service dependencies: %w", queryError)
	}
	defer rows.Close()
	dependencies := make([]ServiceDependency, 0)
	for rows.Next() {
		var dependency ServiceDependency
		if scanError := rows.Scan(&dependency.ServiceName, &dependency.CallCount); scanError != nil {
			return nil, fmt.Errorf("scan service dependency: %w", scanError)
		}
		dependencies = append(dependencies, dependency)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate service dependencies: %w", rowsError)
	}
	return dependencies, nil
}

func (database *Database) ListIncidents(ctx context.Context, projectID string, limit int) ([]Incident, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, primary_service, COALESCE(trace_id, ''), COALESCE(request_id, ''), fingerprint,
			title, COALESCE(error_type, ''), COALESCE(error_message, ''), http_status, severity, status,
			first_seen_at, last_seen_at, occurrence_count
		FROM incidents
		WHERE project_id = $1
		ORDER BY last_seen_at DESC, id DESC
		LIMIT $2`, projectID, limit)
	if queryError != nil {
		return nil, fmt.Errorf("list incidents: %w", queryError)
	}
	defer rows.Close()
	incidents := make([]Incident, 0, limit)
	for rows.Next() {
		var incident Incident
		if scanError := rows.Scan(&incident.ID, &incident.PrimaryService, &incident.TraceID, &incident.RequestID, &incident.Fingerprint,
			&incident.Title, &incident.ErrorType, &incident.ErrorMessage, &incident.HTTPStatus, &incident.Severity, &incident.Status,
			&incident.FirstSeenAt, &incident.LastSeenAt, &incident.OccurrenceCount); scanError != nil {
			return nil, fmt.Errorf("scan incident: %w", scanError)
		}
		incidents = append(incidents, incident)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate incidents: %w", rowsError)
	}
	return incidents, nil
}

func (database *Database) GetIncident(ctx context.Context, projectID, incidentID string) (Incident, error) {
	var incident Incident
	queryError := database.pool.QueryRow(ctx, `
		SELECT id, primary_service, COALESCE(trace_id, ''), COALESCE(request_id, ''), fingerprint,
			title, COALESCE(error_type, ''), COALESCE(error_message, ''), http_status, severity, status,
			first_seen_at, last_seen_at, occurrence_count
		FROM incidents
		WHERE project_id = $1 AND id = $2`, projectID, incidentID).Scan(
		&incident.ID, &incident.PrimaryService, &incident.TraceID, &incident.RequestID, &incident.Fingerprint,
		&incident.Title, &incident.ErrorType, &incident.ErrorMessage, &incident.HTTPStatus, &incident.Severity, &incident.Status,
		&incident.FirstSeenAt, &incident.LastSeenAt, &incident.OccurrenceCount)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return Incident{}, nil
	}
	if queryError != nil {
		return Incident{}, fmt.Errorf("get incident: %w", queryError)
	}
	return incident, nil
}
