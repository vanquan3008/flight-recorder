package incident

import (
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestPolicy(t *testing.T) {
	event := telemetry.Event{ServiceName: "checkout", Operation: "POST /checkout", ErrorType: "PaymentError", ErrorMessage: "declined", HTTPStatus: 500}
	if !Candidate(event) || Fingerprint(event) == "" || Title(event) == "" || Severity(event) != "ERROR" {
		t.Fatal("incident policy rejected a failure")
	}
	event.ErrorMessage = "timeout"
	if Fingerprint(event) == Fingerprint(telemetry.Event{ServiceName: "checkout", Operation: "POST /checkout", ErrorType: "PaymentError", ErrorMessage: "declined", HTTPStatus: 500}) {
		t.Fatal("distinct errors share a fingerprint")
	}
	if Candidate(telemetry.Event{Timestamp: time.Now(), HTTPStatus: 200, Severity: "INFO"}) {
		t.Fatal("healthy event became an incident")
	}
}

func TestStatusTransitions(t *testing.T) {
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
		if got := CanTransition(test.previous, test.next); got != test.valid {
			t.Fatalf("transition %s -> %s = %t, want %t", test.previous, test.next, got, test.valid)
		}
	}
	if status, valid := NormalizeStatus(" resolved "); !valid || status != "RESOLVED" {
		t.Fatalf("normalized status = %q, %t", status, valid)
	}
	if _, valid := NormalizeStatus("closed"); valid {
		t.Fatal("unsupported status was accepted")
	}
}

func TestLogNumericSeverityIsUsedWithCustomText(t *testing.T) {
	for _, testCase := range []struct {
		number    int32
		candidate bool
		severity  string
	}{
		{number: 16, candidate: false, severity: "ERROR"},
		{number: 17, candidate: true, severity: "ERROR"},
		{number: 21, candidate: true, severity: "CRITICAL"},
	} {
		event := telemetry.Event{Signal: telemetry.SignalLog, SeverityNumber: testCase.number, Severity: "custom-level"}
		if Candidate(event) != testCase.candidate || Severity(event) != testCase.severity {
			t.Fatalf("numeric severity %d was not respected", testCase.number)
		}
	}
}

func TestTraceStatusErrorIsIncidentCandidate(t *testing.T) {
	if !Candidate(telemetry.Event{Signal: telemetry.SignalTrace, Status: "ERROR", HTTPStatus: 200}) {
		t.Fatal("non-HTTP span error was excluded")
	}
	if Candidate(telemetry.Event{Signal: telemetry.SignalTrace, Status: "OK", HTTPStatus: 200}) {
		t.Fatal("healthy span produced an incident")
	}
}
