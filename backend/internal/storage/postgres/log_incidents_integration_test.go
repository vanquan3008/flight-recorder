package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestCompactLogIncidentsReuseTimelineAndOutboxWithoutRawLogs(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx, at := context.Background(), time.Now().UTC().Truncate(time.Second)
	if _, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "log-errors", URL: "https://example.com/hook", Events: []string{"incident.opened"}}); err != nil {
		t.Fatal(err)
	}
	event := flowSpan(scope, "api", at, "1111111111111111", "")
	event.Signal, event.Type, event.SeverityNumber = telemetry.SignalLog, telemetry.EventLog, 17
	event.Severity, event.ErrorType, event.ErrorMessage = "ERROR", "DatabaseError", "connection failed"
	event.Attributes = map[string]any{"large": "raw attributes stay in ClickHouse"}
	event.LogBody = "raw body stays in ClickHouse"
	info := event
	info.ID, info.SeverityNumber, info.Severity = telemetry.DeterministicID(at, "info"), 9, "INFO"
	for range 2 {
		if err := database.StoreLogIncidents(ctx, scope, []telemetry.Event{event, info}); err != nil {
			t.Fatal(err)
		}
	}
	for table, want := range map[string]int{"logs": 0, "events": 1, "incident_occurrences": 1, "webhook_deliveries": 1, "incident_evidence": 1} {
		if got := countRows(t, database, "SELECT count(*) FROM "+table+" WHERE project_id=$1", scope.ProjectID); got != want {
			t.Fatalf("%s rows=%d, want %d", table, got, want)
		}
	}
	var attributes string
	if err := database.pool.QueryRow(ctx, "SELECT attributes::text FROM events WHERE project_id=$1", scope.ProjectID).Scan(&attributes); err != nil || attributes != "{}" {
		t.Fatalf("raw attributes copied to PostgreSQL: %s, %v", attributes, err)
	}
	rows, err := database.ListIncidents(ctx, scope.ProjectID, incident.Filter{EnvironmentID: scope.EnvironmentID, Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].OccurrenceCount != 1 {
		t.Fatalf("incident deduplication changed: %+v, %v", rows, err)
	}
	occurrences, err := database.ListIncidentOccurrences(ctx, scope.ProjectID, scope.EnvironmentID, rows[0].ID, 10, time.Time{}, "")
	if err != nil || len(occurrences) != 1 || occurrences[0].ErrorMessage != event.ErrorMessage {
		t.Fatalf("old incident timeline broke: %+v, %v", occurrences, err)
	}
}
