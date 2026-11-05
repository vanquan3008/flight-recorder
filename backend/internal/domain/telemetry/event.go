package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ErrInvalidEvent marks events that storage rejected because of their content.
// Retrying the same event cannot succeed, so it is reported to the client as a
// rejected record instead of a temporary failure.
var ErrInvalidEvent = errors.New("telemetry event cannot be stored")

type EventType string

type Signal string

const (
	SignalTrace Signal = "TRACE"
	SignalLog   Signal = "LOG"
)

const (
	EventHTTPServer EventType = "HTTP_SERVER"
	EventHTTPClient EventType = "HTTP_CLIENT"
	EventLog        EventType = "LOG"
	EventException  EventType = "EXCEPTION"
	EventDatabase   EventType = "DATABASE"
	EventCache      EventType = "CACHE"
	EventMessage    EventType = "MESSAGE"
	EventRPC        EventType = "RPC"
	EventCustom     EventType = "CUSTOM"
)

type Event struct {
	ID                 string
	ProjectID          string
	EnvironmentID      string
	Environment        string
	ServiceName        string
	ServiceVersion     string
	TraceID            string
	SpanID             string
	ParentSpanID       string
	RequestID          string
	Type               EventType
	Timestamp          time.Time
	Duration           time.Duration
	Severity           string
	Status             string
	HTTPStatus         int
	Operation          string
	ErrorType          string
	ErrorMessage       string
	Attributes         map[string]any
	Signal             Signal
	ResourceAttributes map[string]any
	ResourceSchemaURL  string
	ScopeName          string
	ScopeVersion       string
	ScopeSchemaURL     string
	ScopeAttributes    map[string]any
	RecordAttributes   map[string]any

	TraceState        string
	TraceFlags        uint32
	SpanKind          int32
	StatusMessage     string
	DroppedAttributes uint32
	DroppedEvents     uint32
	DroppedLinks      uint32
	SpanEvents        []SpanEvent
	SpanLinks         []SpanLink
	ObservedTimestamp time.Time
	SeverityNumber    int32
	LogBody           any
	LogEventName      string
}

type SpanEvent struct {
	Timestamp         time.Time
	Name              string
	Attributes        map[string]any
	DroppedAttributes uint32
}

type SpanLink struct {
	TraceID           string
	SpanID            string
	TraceState        string
	Flags             uint32
	Attributes        map[string]any
	DroppedAttributes uint32
}

func (event Event) Validate() error {
	if event.ID == "" || event.ProjectID == "" || event.ServiceName == "" {
		return errors.New("Event ID, project ID, and service name are required")
	}
	if event.Timestamp.IsZero() {
		return errors.New("event timestamp is required")
	}
	if event.Type == "" {
		return errors.New("event type is required")
	}
	if event.Duration < 0 {
		return errors.New("event duration must not be negative")
	}
	if event.TraceID != "" && (len(event.TraceID) != 32 || !isHex(event.TraceID)) {
		return errors.New("trace ID must be 16 bytes of hexadecimal")
	}
	if event.SpanID != "" && (len(event.SpanID) != 16 || !isHex(event.SpanID)) {
		return errors.New("span ID must be 8 bytes of hexadecimal")
	}
	if event.ParentSpanID != "" && (len(event.ParentSpanID) != 16 || !isHex(event.ParentSpanID)) {
		return errors.New("parent span ID must be 8 bytes of hexadecimal")
	}
	if event.HTTPStatus != 0 && (event.HTTPStatus < 100 || event.HTTPStatus > 599) {
		return errors.New("HTTP status must be between 100 and 599")
	}
	return nil
}

// DeterministicID returns a UUIDv7 identifier whose timestamp preserves
// event ordering and whose entropy is stable across delivery retries.
func DeterministicID(timestamp time.Time, identity ...string) string {
	milliseconds := timestamp.UnixMilli()
	hash := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	var identifier [16]byte
	identifier[0] = byte(milliseconds >> 40)
	identifier[1] = byte(milliseconds >> 32)
	identifier[2] = byte(milliseconds >> 24)
	identifier[3] = byte(milliseconds >> 16)
	identifier[4] = byte(milliseconds >> 8)
	identifier[5] = byte(milliseconds)
	copy(identifier[6:], hash[:10])
	identifier[6] = 0x70 | identifier[6]&0x0f
	identifier[8] = 0x80 | identifier[8]&0x3f

	encoded := hex.EncodeToString(identifier[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func isHex(value string) bool {
	_, decodeError := hex.DecodeString(value)
	return decodeError == nil
}
