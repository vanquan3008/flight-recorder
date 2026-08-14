package incident

import (
	"testing"

	"github.com/flight-observability/flight/internal/telemetry"
)

func TestDetectHTTPFailureAndStableFingerprint(t *testing.T) {
	event := telemetry.Event{ProjectID: "project", ServiceName: "payments", Type: telemetry.EventHTTPServer, Operation: "POST /payment", HTTPStatus: 500, ErrorMessage: "bank timeout"}
	firstCandidate, detected := Detect(event)
	if !detected {
		t.Fatal("expected incident candidate")
	}
	event.ErrorMessage = "  BANK   timeout "
	secondCandidate, detected := Detect(event)
	if !detected || firstCandidate.Fingerprint != secondCandidate.Fingerprint {
		t.Fatal("fingerprint must ignore case and whitespace")
	}
}

func TestDetectIgnoresSuccessfulEvent(t *testing.T) {
	_, detected := Detect(telemetry.Event{Type: telemetry.EventHTTPServer, HTTPStatus: 200})
	if detected {
		t.Fatal("did not expect incident candidate")
	}
}
