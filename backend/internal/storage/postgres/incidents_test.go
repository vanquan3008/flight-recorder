package postgres

import (
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
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

func TestIncidentStatusTransitions(t *testing.T) {
	for _, test := range []struct {
		previous string
		next     string
		valid    bool
	}{
		{"OPEN", "INVESTIGATING", true},
		{"OPEN", "RESOLVED", true},
		{"INVESTIGATING", "IGNORED", true},
		{"RESOLVED", "OPEN", true},
		{"IGNORED", "OPEN", true},
		{"OPEN", "OPEN", false},
		{"INVESTIGATING", "OPEN", false},
		{"RESOLVED", "IGNORED", false},
	} {
		if got := canTransitionIncidentStatus(test.previous, test.next); got != test.valid {
			t.Fatalf("transition %s -> %s = %t, want %t", test.previous, test.next, got, test.valid)
		}
	}
	if status, valid := NormalizeIncidentStatus(" resolved "); !valid || status != "RESOLVED" {
		t.Fatalf("normalized status = %q, %t", status, valid)
	}
	if _, valid := NormalizeIncidentStatus("closed"); valid {
		t.Fatal("unsupported status was accepted")
	}
}
