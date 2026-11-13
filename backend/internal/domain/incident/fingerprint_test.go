package incident

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestFingerprintGroupsMessagesThatDifferOnlyByValues(t *testing.T) {
	for _, pair := range [][2]string{
		{"user 123 not found", "user 456 not found"},
		{"order 0f8e2c1a-9b3d-4e5f-8a7b-6c5d4e3f2a1b missing", "order 11111111-2222-4333-8444-555555555555 missing"},
		{"deadline at 2026-10-24T08:15:00Z", "deadline at 2026-10-25 09:00:01.5+07:00"},
		{"dial tcp 10.0.0.12:5432: connection refused", "dial tcp 10.0.0.13:6543: connection refused"},
		{"invite for ann@example.com failed", "invite for bob@example.org failed"},
		{"object 0xdeadbeef01 freed", "object 0xc0ffee1234 freed"},
		{"retry  took 1.5s", "retry took 2.25s"},
	} {
		first := telemetry.Event{ServiceName: "api", Operation: "GET /users", ErrorMessage: pair[0], HTTPStatus: 500}
		second := first
		second.ErrorMessage = pair[1]
		if Fingerprint(first) != Fingerprint(second) {
			t.Fatalf("%q and %q opened separate incidents (normalized %q vs %q)", pair[0], pair[1], normalizeMessage(pair[0]), normalizeMessage(pair[1]))
		}
	}
}

func TestFingerprintKeepsDistinctFailuresApart(t *testing.T) {
	base := telemetry.Event{ServiceName: "checkout", Operation: "POST /checkout", ErrorType: "PaymentError", ErrorMessage: "declined", HTTPStatus: 500}
	for name, change := range map[string]func(*telemetry.Event){
		"message":   func(event *telemetry.Event) { event.ErrorMessage = "timeout" },
		"service":   func(event *telemetry.Event) { event.ServiceName = "billing" },
		"operation": func(event *telemetry.Event) { event.Operation = "POST /refund" },
		"type":      func(event *telemetry.Event) { event.ErrorType = "FraudError" },
		"status":    func(event *telemetry.Event) { event.HTTPStatus = 502 },
	} {
		other := base
		change(&other)
		if Fingerprint(base) == Fingerprint(other) {
			t.Fatalf("a different %s shares the fingerprint", name)
		}
	}
}

func TestNormalizeMessageKeepsWordsAndVersionedNames(t *testing.T) {
	for message, want := range map[string]string{
		"deadline exceeded calling decade":   "deadline exceeded calling decade",
		"s3 upload via http2 failed":         "s3 upload via http2 failed",
		"user 42 has 3 sessions":             "user <n> has <n> sessions",
		"  spaces\tand\nnewlines collapse  ": "spaces and newlines collapse",
	} {
		if got := normalizeMessage(message); got != want {
			t.Fatalf("normalizeMessage(%q) = %q, want %q", message, got, want)
		}
	}
}
