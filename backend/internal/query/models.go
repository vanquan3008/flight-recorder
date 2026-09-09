package query

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
	MinDurationNs *int64
	MaxDurationNs *int64
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
