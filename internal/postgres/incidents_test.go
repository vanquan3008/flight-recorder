package postgres

import (
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/telemetry"
)

func TestIncidentFingerprintIsStableAndScoped(t *testing.T) {
	event := telemetry.Event{ServiceName: "checkout", Operation: "POST /checkout", ErrorType: "PaymentError", ErrorMessage: "declined", HTTPStatus: 500}
	if first, second := incidentFingerprint(event), incidentFingerprint(event); first != second {
		t.Fatalf("fingerprint changed: %q != %q", first, second)
	}
	event.ErrorMessage = "timeout"
	if incidentFingerprint(event) == incidentFingerprint(telemetry.Event{ServiceName: "checkout", Operation: "POST /checkout", ErrorType: "PaymentError", ErrorMessage: "declined", HTTPStatus: 500}) {
		t.Fatal("distinct errors share a fingerprint")
	}
}

func TestIncidentCandidate(t *testing.T) {
	if !incidentCandidate(telemetry.Event{HTTPStatus: 500}) || !incidentCandidate(telemetry.Event{Type: telemetry.EventException}) || !incidentCandidate(telemetry.Event{Severity: "FATAL"}) {
		t.Fatal("expected failure candidates")
	}
	if incidentCandidate(telemetry.Event{Timestamp: time.Now(), HTTPStatus: 200, Severity: "INFO"}) {
		t.Fatal("healthy event became an incident")
	}
}
