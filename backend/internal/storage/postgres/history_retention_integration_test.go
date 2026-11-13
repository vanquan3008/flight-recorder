package postgres

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// seedIncident stores one failing span at `at` under a message no other test
// incident shares, and returns the incident ID it opened.
func seedIncident(t *testing.T, database *Database, scope config.IngestionConfig, word string, at time.Time) string {
	t.Helper()
	spanID := hex.EncodeToString([]byte((word + "________")[:8])) // 16 hex characters, unique per word
	event := flowSpan(scope, "api", at, spanID, "")
	event.Status, event.ErrorMessage = "ERROR", word+" failure"
	if err := database.StoreEvents(context.Background(), scope, []telemetry.Event{event}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := database.pool.QueryRow(context.Background(), "SELECT id::text FROM incidents WHERE project_id = $1 AND error_message = $2", scope.ProjectID, word+" failure").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countRows(t *testing.T, database *Database, query string, args ...any) int {
	t.Helper()
	var count int
	if err := database.pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestIncidentRetentionKeepsActiveWorkAndLeavesNoOrphans(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	actor := testUser(t, database)
	old := time.Now().UTC().Add(-120 * 24 * time.Hour).Truncate(time.Second)
	incidents := map[string]string{}
	for _, word := range []string{"alpha", "beta", "gamma"} {
		incidents[word] = seedIncident(t, database, scope, word, old)
	}
	for _, word := range []string{"alpha", "gamma"} {
		if _, err := database.UpdateIncidentStatus(ctx, scope.ProjectID, scope.EnvironmentID, incidents[word], "RESOLVED", auth.Actor{UserID: actor.ID}); err != nil {
			t.Fatal(err)
		}
	}
	// alpha: resolved long ago. beta: still open. gamma: resolved just now.
	if _, err := database.pool.Exec(ctx, "UPDATE incidents SET updated_at = $2 WHERE id = $1 OR id = $3", incidents["alpha"], old, incidents["beta"]); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	deleted, err := database.DeleteIncidentsBefore(ctx, cutoff, 100)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d error=%v, want only the long-resolved incident", deleted, err)
	}
	var remaining []string
	rows, err := database.pool.Query(ctx, "SELECT error_message FROM incidents WHERE project_id = $1 ORDER BY error_message", scope.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			t.Fatal(err)
		}
		remaining = append(remaining, message)
	}
	rows.Close()
	if strings.Join(remaining, ",") != "beta failure,gamma failure" {
		t.Fatalf("remaining incidents = %v; an open incident and a recently resolved one must survive", remaining)
	}
	for _, table := range []string{"incident_occurrences", "incident_events", "incident_evidence", "incident_status_changes"} {
		orphans := countRows(t, database, "SELECT count(*) FROM "+table+" WHERE project_id = $1 AND incident_id NOT IN (SELECT id FROM incidents WHERE project_id = $1)", scope.ProjectID)
		if orphans != 0 {
			t.Fatalf("%s kept %d rows of a deleted incident", table, orphans)
		}
	}
	if kept := countRows(t, database, "SELECT count(*) FROM incident_evidence WHERE project_id = $1", scope.ProjectID); kept != 2 {
		t.Fatalf("evidence rows = %d, want the 2 that belong to surviving incidents", kept)
	}
}

func TestAuditAndDeliveryRetentionKeepLiveWork(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	actor := testUser(t, database)
	day := 24 * time.Hour
	for index, age := range []time.Duration{200 * day, 10 * day} {
		if _, err := database.pool.Exec(ctx, `INSERT INTO audit_events (id, project_id, actor_user_id, action, target_type, target_id, created_at) VALUES ($1, $2, $3, 'member.added', 'user', $5, $4)`,
			telemetry.DeterministicID(time.Now(), "audit", scope.ProjectID, fmt.Sprint(index)), scope.ProjectID, actor.ID, time.Now().UTC().Add(-age), actor.ID); err != nil {
			t.Fatal(err)
		}
	}
	if deleted, err := database.DeleteAuditBefore(ctx, time.Now().UTC().Add(-180*day), 100); err != nil || deleted != 1 {
		t.Fatalf("audit deleted=%d error=%v, want the 200-day-old event only", deleted, err)
	}

	webhook, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "retention", URL: "https://example.com/hook", Events: []string{"incident.opened"}})
	if err != nil {
		t.Fatal(err)
	}
	for index, delivery := range []struct {
		status string
		age    time.Duration
	}{{"succeeded", 40 * day}, {"failed", 40 * day}, {"pending", 40 * day}, {"sending", 40 * day}, {"succeeded", 5 * day}} {
		stamp := time.Now().UTC().Add(-delivery.age)
		if _, err := database.pool.Exec(ctx, `INSERT INTO webhook_deliveries (id, project_id, webhook_id, event_type, payload, status, attempt, max_attempts, next_attempt_at, created_at, updated_at)
			VALUES ($1, $2, $3, 'incident.opened', '{}', $4, 0, 3, $5, $5, $5)`,
			telemetry.DeterministicID(time.Now(), "delivery", scope.ProjectID, fmt.Sprint(index)), scope.ProjectID, webhook.Webhook.ID, delivery.status, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if deleted, err := database.DeleteDeliveriesBefore(ctx, time.Now().UTC().Add(-30*day), 100); err != nil || deleted != 2 {
		t.Fatalf("deliveries deleted=%d error=%v, want only the two old finished ones", deleted, err)
	}
	if live := countRows(t, database, "SELECT count(*) FROM webhook_deliveries WHERE project_id = $1 AND status IN ('pending', 'sending')", scope.ProjectID); live != 2 {
		t.Fatalf("live deliveries = %d, want 2: unfinished work must never expire", live)
	}
}

func TestConcurrentRetentionRunsDoNotConflict(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	actor := testUser(t, database)
	for index := range 20 {
		if _, err := database.pool.Exec(ctx, `INSERT INTO audit_events (id, project_id, actor_user_id, action, target_type, target_id, created_at) VALUES ($1, $2, $3, 'member.added', 'user', $5, $4)`,
			telemetry.DeterministicID(time.Now(), "concurrent", scope.ProjectID, fmt.Sprint(index)), scope.ProjectID, actor.ID, time.Now().UTC().Add(-300*24*time.Hour), actor.ID); err != nil {
			t.Fatal(err)
		}
	}
	var (
		wait    sync.WaitGroup
		mutex   sync.Mutex
		total   int64
		failure error
	)
	for range 4 { // four replicas sweeping the same rows in small batches
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				deleted, err := database.DeleteAuditBefore(ctx, time.Now().UTC().Add(-180*24*time.Hour), 3)
				mutex.Lock()
				total += deleted
				if err != nil {
					failure = err
				}
				mutex.Unlock()
				if err != nil || deleted == 0 {
					return
				}
			}
		}()
	}
	wait.Wait()
	if failure != nil || total != 20 {
		t.Fatalf("deleted %d rows, error %v; concurrent sweeps must remove each row exactly once", total, failure)
	}
}
