package logs

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestPersistRoutesAndWaitsForEveryStore(t *testing.T) {
	info := telemetry.Event{ID: "info", Signal: telemetry.SignalLog}
	failure := telemetry.Event{ID: "error", Signal: telemetry.SignalLog, SeverityNumber: 17}
	span := telemetry.Event{ID: "span", Signal: telemetry.SignalTrace, Status: "OK"}
	failedSpan := telemetry.Event{ID: "failed-span", Signal: telemetry.SignalTrace, Status: "ERROR"}
	want := []string{"logs:info", "logs:error", "spans:span", "spans:failed-span", "failures:failed-span", "failures:error"}
	for _, failAt := range []string{"", "logs", "spans", "failures"} {
		t.Run(failAt, func(t *testing.T) {
			var calls []string
			problem := errors.New("store unavailable")
			writer := func(name string) WriteEvents {
				return func(_ context.Context, events []telemetry.Event) error {
					for _, event := range events {
						calls = append(calls, name+":"+event.ID)
					}
					if name == failAt {
						return problem
					}
					return nil
				}
			}
			err := Persist(context.Background(), []telemetry.Event{info, span, failedSpan, failure}, writer("logs"), writer("spans"), writer("failures"))
			expected := want
			if failAt != "" {
				if !errors.Is(err, problem) {
					t.Fatalf("acknowledged a failed %s write: %v", failAt, err)
				}
				expected = want[:map[string]int{"logs": 2, "spans": 4, "failures": 6}[failAt]]
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, expected) {
				t.Fatalf("calls %v, want %v", calls, expected)
			}
		})
	}
}

func TestHealthyTelemetryNeverReachesTheIncidentStore(t *testing.T) {
	incidents := func(context.Context, []telemetry.Event) error {
		t.Fatal("healthy telemetry reached PostgreSQL")
		return nil
	}
	telemetryStore := func(context.Context, []telemetry.Event) error { return nil }
	events := []telemetry.Event{{Signal: telemetry.SignalLog, SeverityNumber: 9}, {Signal: telemetry.SignalTrace, Status: "OK"}}
	if err := Persist(context.Background(), events, telemetryStore, telemetryStore, incidents); err != nil {
		t.Fatal(err)
	}
}
