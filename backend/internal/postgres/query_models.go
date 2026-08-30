package postgres

import (
	"encoding/json"
	"time"
)

type TraceFilter struct {
	EnvironmentID string
	Service       string
	Operation     string
	Status        string
	TraceID       string
	From          time.Time
	To            time.Time
	Before        time.Time
	CursorID      string
	Limit         int
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
	EnvironmentID string
	Service       string
	Severity      string
	TraceID       string
	SpanID        string
	Search        string
	From          time.Time
	To            time.Time
	Before        time.Time
	CursorID      string
	Limit         int
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

type IncidentFilter struct {
	EnvironmentID string
	Service       string
	Severity      string
	Status        string
	From          time.Time
	To            time.Time
	Before        time.Time
	CursorID      string
	Limit         int
}

type IncidentOccurrence struct {
	ID           string    `json:"id"`
	EventID      string    `json:"event_id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ServiceName  string    `json:"service_name"`
	TraceID      string    `json:"trace_id,omitempty"`
	EventType    string    `json:"event_type"`
	Operation    string    `json:"operation,omitempty"`
	ErrorType    string    `json:"error_type,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	HTTPStatus   *int      `json:"http_status,omitempty"`
}

type IncidentEvent struct {
	EventID      string    `json:"event_id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ServiceName  string    `json:"service_name"`
	TraceID      string    `json:"trace_id,omitempty"`
	EventType    string    `json:"event_type"`
	Operation    string    `json:"operation,omitempty"`
	ErrorType    string    `json:"error_type,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	HTTPStatus   *int      `json:"http_status,omitempty"`
}

type IncidentStatusChange struct {
	ID             string    `json:"id"`
	PreviousStatus string    `json:"previous_status"`
	Status         string    `json:"status"`
	ActorKeyID     string    `json:"actor_key_id"`
	ChangedAt      time.Time `json:"changed_at"`
}
