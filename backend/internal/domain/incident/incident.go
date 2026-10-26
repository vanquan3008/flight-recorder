package incident

import (
	"errors"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

var (
	ErrNotFound          = errors.New("incident not found")
	ErrInvalidTransition = errors.New("invalid incident status transition")
)

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

type Filter struct {
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

type Occurrence struct {
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

type Event struct {
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

type StatusChange struct {
	ID             string    `json:"id"`
	PreviousStatus string    `json:"previous_status"`
	Status         string    `json:"status"`
	ActorKeyID     string    `json:"actor_key_id"`
	ChangedAt      time.Time `json:"changed_at"`
}

func NormalizeStatus(value string) (string, bool) {
	status := strings.ToUpper(strings.TrimSpace(value))
	switch status {
	case "OPEN", "INVESTIGATING", "RESOLVED", "IGNORED":
		return status, true
	default:
		return "", false
	}
}

func CanTransition(previous, next string) bool {
	switch previous {
	case "OPEN":
		return next == "INVESTIGATING" || next == "RESOLVED" || next == "IGNORED"
	case "INVESTIGATING":
		return next == "RESOLVED" || next == "IGNORED"
	case "RESOLVED", "IGNORED":
		return next == "OPEN"
	default:
		return false
	}
}

func Candidate(event telemetry.Event) bool {
	if event.Signal == telemetry.SignalTrace && event.Status == "ERROR" {
		return true
	}
	if event.Signal == telemetry.SignalLog && event.SeverityNumber >= 17 {
		return true
	}
	if event.HTTPStatus >= 500 || event.Type == telemetry.EventException {
		return true
	}
	severity := strings.ToUpper(event.Severity)
	return strings.Contains(severity, "ERROR") || strings.Contains(severity, "FATAL")
}

func Title(event telemetry.Event) string {
	if event.Operation != "" {
		return event.ServiceName + " " + event.Operation + " failed"
	}
	if event.ErrorType != "" {
		return event.ServiceName + " " + event.ErrorType
	}
	return event.ServiceName + " failure"
}

func Severity(event telemetry.Event) string {
	if (event.Signal == telemetry.SignalLog && event.SeverityNumber >= 21) || strings.Contains(strings.ToUpper(event.Severity), "FATAL") {
		return "CRITICAL"
	}
	return "ERROR"
}
