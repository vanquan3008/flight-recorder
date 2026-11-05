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
	span := telemetry.Event{ID: "span", Signal: telemetry.SignalTrace}
	want := []string{"logs:info", "logs:error", "other:span", "failures:error"}
	for _, failAt := range []string{"", "logs", "other", "failures"} {
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
			err := Persist(context.Background(), []telemetry.Event{info, span, failure}, writer("logs"), writer("other"), writer("failures"))
			expected := want
			if failAt != "" {
				if !errors.Is(err, problem) {
					t.Fatalf("acknowledged a failed %s write: %v", failAt, err)
				}
				end := map[string]int{"logs": 2, "other": 3, "failures": 4}[failAt]
				expected = want[:end]
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, expected) {
				t.Fatalf("calls %v, want %v", calls, expected)
			}
		})
	}
}

func TestNormalLogsNeverWritePostgres(t *testing.T) {
	pg := func(context.Context, []telemetry.Event) error { t.Fatal("normal log reached PostgreSQL"); return nil }
	ch := func(context.Context, []telemetry.Event) error { return nil }
	if err := Persist(context.Background(), []telemetry.Event{{Signal: telemetry.SignalLog, SeverityNumber: 9}}, ch, pg, pg); err != nil {
		t.Fatal(err)
	}
}
