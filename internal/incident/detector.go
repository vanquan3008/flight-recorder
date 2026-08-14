package incident

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/flight-observability/flight/internal/telemetry"
	"strings"
)

type Candidate struct {
	ProjectID    string
	Environment  string
	ServiceName  string
	TraceID      string
	RequestID    string
	Fingerprint  string
	Title        string
	ErrorType    string
	ErrorMessage string
	HTTPStatus   int
	Severity     string
}

func Detect(event telemetry.Event) (Candidate, bool) {
	normalizedSeverity := strings.ToUpper(strings.TrimSpace(event.Severity))
	isFailure := event.HTTPStatus >= 500 || event.Type == telemetry.EventException || normalizedSeverity == "ERROR" || normalizedSeverity == "FATAL"
	if !isFailure {
		return Candidate{}, false
	}

	incidentTitle := strings.TrimSpace(event.ErrorMessage)
	if incidentTitle == "" {
		incidentTitle = strings.TrimSpace(event.Operation)
	}
	if incidentTitle == "" {
		incidentTitle = "Unhandled production failure"
	}

	return Candidate{
		ProjectID:    event.ProjectID,
		Environment:  event.Environment,
		ServiceName:  event.ServiceName,
		TraceID:      event.TraceID,
		RequestID:    event.RequestID,
		Fingerprint:  fingerprint(event),
		Title:        incidentTitle,
		ErrorType:    event.ErrorType,
		ErrorMessage: event.ErrorMessage,
		HTTPStatus:   event.HTTPStatus,
		Severity:     normalizedSeverity,
	}, true
}

func fingerprint(event telemetry.Event) string {
	fingerprintFields := []string{
		event.ServiceName,
		event.Operation,
		event.ErrorType,
		event.ErrorMessage,
	}
	for fieldIndex := range fingerprintFields {
		fingerprintFields[fieldIndex] = strings.ToLower(strings.Join(strings.Fields(fingerprintFields[fieldIndex]), " "))
	}
	fingerprintHash := sha256.Sum256([]byte(strings.Join(fingerprintFields, "\x00")))
	return hex.EncodeToString(fingerprintHash[:])
}
