package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"

	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// resolvedIncident stores one failure, has an operator resolve it, and returns
// the incident ID and the time of the resolve.
func resolvedIncident(t *testing.T, database *Database, scope config.IngestionConfig, failedAt time.Time) (incidentID, actor string) {
	t.Helper()
	ctx := context.Background()
	principal := testUser(t, database)
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{failingSpan(scope, failedAt, "1111111111111111", "payment declined")}); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, "SELECT id FROM incidents WHERE project_id = $1", scope.ProjectID).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateIncidentStatus(ctx, scope.ProjectID, scope.EnvironmentID, incidentID, "RESOLVED", auth.Actor{UserID: principal.ID}); err != nil {
		t.Fatal(err)
	}
	return incidentID, principal.ID
}

func incidentStatus(t *testing.T, database *Database, incidentID string) string {
	t.Helper()
	var status string
	if err := database.pool.QueryRow(context.Background(), "SELECT status FROM incidents WHERE id = $1", incidentID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestResolvedIncidentReopensWhenTheFailureReturns(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	if _, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "pager", URL: "https://example.com/hook", Events: []string{"incident.opened"}}); err != nil {
		t.Fatal(err)
	}
	failedAt := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
	incidentID, _ := resolvedIncident(t, database, scope, failedAt)
	var deliveriesBefore int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE project_id = $1", scope.ProjectID).Scan(&deliveriesBefore); err != nil {
		t.Fatal(err)
	}

	// The same failure, after the resolve, inside the deduplication window.
	again := failingSpan(scope, time.Now().UTC().Add(time.Minute), "2222222222222222", "payment declined")
	for attempt := range 2 { // the second attempt models an exporter retry
		if err := database.StoreEvents(ctx, scope, []telemetry.Event{again}); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}

	if got := incidentStatus(t, database, incidentID); got != "OPEN" {
		t.Fatalf("status = %s, want OPEN: a resolved incident that failed again stayed resolved", got)
	}
	changes, err := database.ListIncidentStatusChanges(ctx, scope.ProjectID, scope.EnvironmentID, incidentID, 10, time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Status != "OPEN" || changes[0].PreviousStatus != "RESOLVED" || changes[0].ActorUserID != "" {
		t.Fatalf("history = %+v, want the operator's resolve followed by one system reopen", changes)
	}
	var deliveriesAfter int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE project_id = $1", scope.ProjectID).Scan(&deliveriesAfter); err != nil {
		t.Fatal(err)
	}
	if deliveriesAfter != deliveriesBefore+1 {
		t.Fatalf("deliveries %d -> %d: a reopen must notify once, even when the batch is retried", deliveriesBefore, deliveriesAfter)
	}
}

// Two resolves by the same operator carry identical webhook payloads; each one
// is a separate transition and must queue its own delivery.
func TestRepeatedTransitionsEachNotify(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	if _, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "pager", URL: "https://example.com/hook", Events: []string{"incident.resolved"}}); err != nil {
		t.Fatal(err)
	}
	incidentID, actor := resolvedIncident(t, database, scope, time.Now().UTC().Truncate(time.Second).Add(-10*time.Minute))
	for _, status := range []string{"OPEN", "RESOLVED"} {
		if _, err := database.UpdateIncidentStatus(ctx, scope.ProjectID, scope.EnvironmentID, incidentID, status, auth.Actor{UserID: actor}); err != nil {
			t.Fatal(err)
		}
	}
	var resolved int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE project_id = $1 AND event_type = 'incident.resolved'", scope.ProjectID).Scan(&resolved); err != nil {
		t.Fatal(err)
	}
	if resolved != 2 {
		t.Fatalf("incident.resolved deliveries = %d, want 2: the second resolve was absorbed as a duplicate", resolved)
	}
}

func TestLateEventsFromBeforeTheResolveDoNotReopen(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	failedAt := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
	incidentID, _ := resolvedIncident(t, database, scope, failedAt)

	late := failingSpan(scope, failedAt.Add(time.Minute), "3333333333333333", "payment declined")
	if err := database.StoreEvents(context.Background(), scope, []telemetry.Event{late}); err != nil {
		t.Fatal(err)
	}
	if got := incidentStatus(t, database, incidentID); got != "RESOLVED" {
		t.Fatalf("status = %s: an event older than the resolve undid it", got)
	}
}

func TestIgnoredIncidentStaysIgnored(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	failedAt := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
	incidentID, actor := resolvedIncident(t, database, scope, failedAt)
	if _, err := database.UpdateIncidentStatus(ctx, scope.ProjectID, scope.EnvironmentID, incidentID, "OPEN", auth.Actor{UserID: actor}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateIncidentStatus(ctx, scope.ProjectID, scope.EnvironmentID, incidentID, "IGNORED", auth.Actor{UserID: actor}); err != nil {
		t.Fatal(err)
	}
	again := failingSpan(scope, time.Now().UTC().Add(time.Minute), "4444444444444444", "payment declined")
	if err := database.StoreEvents(ctx, scope, []telemetry.Event{again}); err != nil {
		t.Fatal(err)
	}
	if got := incidentStatus(t, database, incidentID); got != "IGNORED" {
		t.Fatalf("status = %s: an ignored incident was reopened by new occurrences", got)
	}
}
