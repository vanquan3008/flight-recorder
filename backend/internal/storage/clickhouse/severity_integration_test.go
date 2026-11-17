package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestLogSeverityGroupsIntegration(t *testing.T) {
	store, ctx := integrationStore(t, 0), context.Background()
	at := time.Now().UTC()
	cases := []struct {
		text   string
		number int32
		group  string
	}{
		{"Error", 0, "ERROR"}, {"ERROR4", 0, "ERROR"}, {"custom", 20, "ERROR"},
		{"WARNING", 0, "WARN"}, {"Error", 9, "INFO"}, {" ", 0, "INFO"},
		{"fatal", 24, "FATAL"}, {"custom", 0, "CUSTOM"},
	}
	var events []telemetry.Event
	for index, tc := range cases {
		moment := at.Add(time.Duration(index))
		event := testLog(moment, moment.Format(time.RFC3339Nano))
		event.Severity, event.SeverityNumber = tc.text, tc.number
		events = append(events, event)
	}
	if err := store.StoreLogs(ctx, testProject, testEnvironment, events); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{"ERROR", "error2", "WARN", "WARNING", "INFO", "FATAL", "CUSTOM"} {
		t.Run(filter, func(t *testing.T) {
			group := filter
			if filter == "error2" {
				group = "ERROR"
			}
			if filter == "WARNING" {
				group = "WARN"
			}
			wanted := make(map[string]string)
			for index, tc := range cases {
				if tc.group == group || (group == "CUSTOM" && tc.text == "custom") {
					wanted[events[index].ID] = tc.text
				}
			}
			rows, err := store.ListLogs(ctx, testProject, query.LogFilter{EnvironmentID: testEnvironment, Severity: filter, Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(wanted) {
				t.Fatalf("got %d logs want %d", len(rows), len(wanted))
			}
			for _, row := range rows {
				text, ok := wanted[row.EventID]
				if !ok || row.SeverityText != text {
					t.Fatalf("wrong log or changed exporter text: %+v", row)
				}
			}
		})
	}
}
