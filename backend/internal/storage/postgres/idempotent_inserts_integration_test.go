package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// raceIdenticalRows runs write from many connections at once for each round,
// every connection inserting the very same row (same primary key, same natural
// key). A statement that is meant to be idempotent must let all of them
// succeed and leave one row.
func raceIdenticalRows(t *testing.T, database *Database, rounds int, write func(ctx context.Context, queries *sqlcstore.Queries, round int) error) {
	t.Helper()
	const writers = 12
	queries := sqlcstore.New(database.pool)
	for round := range rounds {
		var wait sync.WaitGroup
		start := make(chan struct{})
		failures := make(chan error, writers)
		for range writers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				if err := write(context.Background(), queries, round); err != nil {
					failures <- err
				}
			}()
		}
		close(start)
		wait.Wait()
		close(failures)
		for err := range failures {
			t.Fatalf("round %d: %v; an idempotent insert must absorb a concurrent identical row", round, err)
		}
	}
}

func TestIdempotentInsertsAbsorbConcurrentIdenticalRows(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	const rounds = 150
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)

	t.Run("service", func(t *testing.T) {
		raceIdenticalRows(t, database, rounds, func(ctx context.Context, queries *sqlcstore.Queries, round int) error {
			return queries.EnsureService(ctx, sqlcstore.EnsureServiceParams{
				ID:        telemetry.DeterministicID(at, "race-service", scope.ProjectID, fmt.Sprint(round)),
				ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, Name: fmt.Sprintf("race-%d", round),
			})
		})
		if got := countRows(t, database, "SELECT count(*) FROM services WHERE project_id = $1 AND name LIKE 'race-%'", scope.ProjectID); got != rounds {
			t.Fatalf("services = %d, want one per round (%d)", got, rounds)
		}
	})

	t.Run("span", func(t *testing.T) {
		raceIdenticalRows(t, database, rounds, func(ctx context.Context, queries *sqlcstore.Queries, round int) error {
			return queries.InsertSpan(ctx, sqlcstore.InsertSpanParams{
				EventID:   telemetry.DeterministicID(at, "race-span", scope.ProjectID, fmt.Sprint(round)),
				ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID,
				TraceID: pagingTraceID, SpanID: fmt.Sprintf("%016x", round+1), SpanName: "work", StartedAt: at, EndedAt: at.Add(time.Millisecond),
				StatusCode: "OK", ServiceName: "api",
				ResourceAttributes: []byte("{}"), ScopeAttributes: []byte("{}"), SpanAttributes: []byte("{}"), SpanEvents: []byte("[]"), SpanLinks: []byte("[]"),
			})
		})
		if got := countRows(t, database, "SELECT count(*) FROM spans WHERE project_id = $1", scope.ProjectID); got != rounds {
			t.Fatalf("spans = %d, want one per round (%d)", got, rounds)
		}
	})

	t.Run("webhook delivery", func(t *testing.T) {
		hook, err := database.CreateWebhook(context.Background(), scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "race", URL: "https://example.com/hook", Events: []string{"incident.opened"}})
		if err != nil {
			t.Fatal(err)
		}
		webhookID := hook.Webhook.ID
		raceIdenticalRows(t, database, rounds, func(ctx context.Context, queries *sqlcstore.Queries, round int) error {
			return queries.EnqueueWebhookDelivery(ctx, sqlcstore.EnqueueWebhookDeliveryParams{
				ID:        telemetry.DeterministicID(at, "race-delivery", scope.ProjectID, fmt.Sprint(round)),
				ProjectID: scope.ProjectID, WebhookID: webhookID, EventType: "incident.opened", Payload: []byte("{}"),
			})
		})
		if got := countRows(t, database, "SELECT count(*) FROM webhook_deliveries WHERE project_id = $1", scope.ProjectID); got != rounds {
			t.Fatalf("deliveries = %d, want one per round (%d)", got, rounds)
		}
	})
}

// Replicas started together run the same startup steps against a database that
// may not hold their project or bootstrap key yet.
func TestReplicasStartingTogetherProvisionTheSameScopeAndKey(t *testing.T) {
	database, _ := newFlowTestDatabase(t)
	const rounds = 60
	var replicas [4]*Database
	for index := range replicas {
		replicas[index] = openReplica(t)
	}
	for round := range rounds {
		at := time.Now().UTC()
		scope := config.IngestionConfig{
			ProjectID: telemetry.DeterministicID(at, "startup-project", fmt.Sprint(round), t.Name()), ProjectSlug: fmt.Sprintf("startup-%d-%d", round, at.UnixNano()), ProjectName: "Startup race",
			EnvironmentID: telemetry.DeterministicID(at, "startup-environment", fmt.Sprint(round), t.Name()), EnvironmentName: "test",
		}
		key := fmt.Sprintf("startup-race-%d-bootstrap-key-%020d", at.UnixNano(), round) // key_hash is unique across projects
		t.Cleanup(func() { cleanupStartupScope(t, database, scope.ProjectID) })
		for _, err := range runOnReplicas(replicas[:], func(_ int, replica *Database) error {
			if err := replica.EnsureIngestionScope(context.Background(), scope); err != nil {
				return err
			}
			return replica.EnsureBootstrapAPIKey(context.Background(), scope.ProjectID, key)
		}) {
			if err != nil {
				t.Fatalf("round %d: %v; replicas starting together must all come up", round, err)
			}
		}
		if got := countRows(t, database, "SELECT count(*) FROM api_keys WHERE project_id = $1", scope.ProjectID); got != 1 {
			t.Fatalf("round %d: %d bootstrap keys, want 1", round, got)
		}
	}
}

func cleanupStartupScope(t *testing.T, database *Database, projectID string) {
	t.Helper()
	if _, err := database.pool.Exec(context.Background(), "DELETE FROM projects WHERE id = $1", projectID); err != nil {
		t.Fatal(err)
	}
}
