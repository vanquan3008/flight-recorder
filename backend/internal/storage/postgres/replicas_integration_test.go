package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// openReplica opens another application instance's connection pool onto the
// same database, with the same webhook key, the way a second replica would.
func openReplica(t *testing.T) *Database {
	t.Helper()
	replica, err := OpenWithWebhookEncryptionKey(context.Background(), config.DatabaseConfig{
		URL: os.Getenv("TRACEPILOT_DATABASE_URL"), MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	}, hex.EncodeToString([]byte(strings.Repeat("w", 32))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(replica.Close)
	return replica
}

// runOnReplicas runs work on every replica at the same time and returns each result.
func runOnReplicas(replicas []*Database, work func(index int, replica *Database) error) []error {
	results := make([]error, len(replicas))
	var wait sync.WaitGroup
	start := make(chan struct{})
	for index, replica := range replicas {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results[index] = work(index, replica)
		}()
	}
	close(start)
	wait.Wait()
	return results
}

func isDeadlock(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "40P01"
}

// storeLikeThePipeline retries what the ingestion pipeline retries: a
// transient error such as a detected deadlock.
func storeLikeThePipeline(replica *Database, scope config.IngestionConfig, events []telemetry.Event) error {
	var err error
	for range 3 {
		if err = replica.StoreEvents(context.Background(), scope, events); !isDeadlock(err) {
			return err
		}
	}
	return err
}

func TestReplicasIngestingTheSameRetriedBatchStoreItOnce(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	if _, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "opened", URL: "https://example.com/hook", Events: []string{"incident.opened"}}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	var batch []telemetry.Event
	for index := range 40 {
		event := flowSpan(scope, "api", at.Add(time.Duration(index)*time.Millisecond), fmt.Sprintf("%016x", index+1), "")
		event.Status, event.ErrorMessage = "ERROR", fmt.Sprintf("order %d not found", index)
		batch = append(batch, event)
	}
	reversed := slices.Clone(batch)
	slices.Reverse(reversed)
	replicas := []*Database{openReplica(t), openReplica(t), openReplica(t)}
	// One exporter retried through three replicas, two of them seeing the batch in another order.
	for _, err := range runOnReplicas(replicas, func(index int, replica *Database) error {
		if index == 1 {
			return storeLikeThePipeline(replica, scope, reversed)
		}
		return storeLikeThePipeline(replica, scope, batch)
	}) {
		if err != nil {
			t.Fatal(err)
		}
	}

	var events, occurrences, incidents, occurrenceCount, deliveries int
	for query, target := range map[string]*int{
		"SELECT count(*) FROM events WHERE project_id = $1":                              &events,
		"SELECT count(*) FROM incident_occurrences WHERE project_id = $1":                &occurrences,
		"SELECT count(*) FROM incidents WHERE project_id = $1":                           &incidents,
		"SELECT COALESCE(sum(occurrence_count), 0) FROM incidents WHERE project_id = $1": &occurrenceCount,
		"SELECT count(*) FROM webhook_deliveries WHERE project_id = $1":                  &deliveries,
	} {
		if err := database.pool.QueryRow(ctx, query, scope.ProjectID).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if events != 40 || occurrences != 40 || incidents != 1 || occurrenceCount != 40 || deliveries != 1 {
		t.Fatalf("events=%d occurrences=%d incidents=%d counted=%d deliveries=%d; three replicas must leave exactly one copy and one notification",
			events, occurrences, incidents, occurrenceCount, deliveries)
	}
}

func TestOutboxClaimsAreDisjointAcrossReplicasAndRecoverAfterACrash(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	webhook, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "claims", URL: "https://example.com/hook", Events: []string{"incident.opened"}})
	if err != nil {
		t.Fatal(err)
	}
	mine := map[string]bool{}
	for index := range 12 {
		id := telemetry.DeterministicID(time.Now(), "claim", scope.ProjectID, fmt.Sprint(index))
		mine[id] = true
		if _, err := database.pool.Exec(ctx, `INSERT INTO webhook_deliveries (id, project_id, webhook_id, event_type, payload, status, attempt, max_attempts, next_attempt_at, created_at, updated_at)
			VALUES ($1, $2, $3, 'incident.opened', '{}', 'pending', 0, 3, now() - interval '1 minute', now(), now())`, id, scope.ProjectID, webhook.Webhook.ID); err != nil {
			t.Fatal(err)
		}
	}
	claimAll := func(replicas []*Database) (claimed map[string]int) {
		claimed = map[string]int{}
		var mutex sync.Mutex
		runOnReplicas(replicas, func(_ int, replica *Database) error {
			for {
				batch, err := replica.ClaimDueDeliveries(ctx, 2)
				if err != nil {
					return err
				}
				mutex.Lock()
				for _, delivery := range batch {
					if mine[delivery.ID] {
						claimed[delivery.ID]++
					}
				}
				mutex.Unlock()
				if len(batch) == 0 {
					return nil
				}
			}
		})
		return claimed
	}

	replicas := []*Database{openReplica(t), openReplica(t), openReplica(t), openReplica(t)}
	claimed := claimAll(replicas)
	if len(claimed) != 12 {
		t.Fatalf("claimed %d of 12 deliveries", len(claimed))
	}
	for id, times := range claimed {
		if times != 1 {
			t.Fatalf("delivery %s was claimed by %d replicas at once", id, times)
		}
	}
	if again := claimAll(replicas); len(again) != 0 {
		t.Fatalf("%d deliveries were handed out again while their lease was live", len(again))
	}

	// The replica that held the leases crashed: they age past the lease and another claims them.
	if _, err := database.pool.Exec(ctx, "UPDATE webhook_deliveries SET updated_at = now() - interval '7 minutes' WHERE project_id = $1 AND status = 'sending'", scope.ProjectID); err != nil {
		t.Fatal(err)
	}
	var finished string
	for id := range mine {
		finished = id
		break
	}
	if err := database.RecordDeliveryAttempt(ctx, finished, webhook.Webhook.ID, scope.ProjectID, 200, "", true); err != nil {
		t.Fatal(err)
	}
	recovered := claimAll(replicas)
	if len(recovered) != 11 || recovered[finished] != 0 {
		t.Fatalf("recovered %d deliveries (finished one reclaimed: %t), want the 11 unfinished", len(recovered), recovered[finished] != 0)
	}
}

func TestReplicasDetectingTheSameWindowOpenOneIncident(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	if _, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{Name: "detected", URL: "https://example.com/hook", Events: []string{"incident.opened"}}); err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC().Truncate(time.Minute)
	evidence := []analysis.Evidence{
		{SignalType: "span", EntityType: "service", EntityID: "service:api", MetricName: "error_rate", Direction: "increase", ObservedValue: 0.5, BaselineValue: 0.01, ChangeRatio: 50, WindowStartedAt: end.Add(-5 * time.Minute), WindowEndedAt: end, Summary: "error rate rose"},
		{SignalType: "span", EntityType: "service", EntityID: "service:api", MetricName: "latency", Direction: "increase", ObservedValue: 900, BaselineValue: 100, ChangeRatio: 9, WindowStartedAt: end.Add(-5 * time.Minute), WindowEndedAt: end, Summary: "latency rose"},
	}
	replicas := []*Database{openReplica(t), openReplica(t), openReplica(t)}
	for _, err := range runOnReplicas(replicas, func(_ int, replica *Database) error {
		return replica.PersistObservabilityIncident(ctx, scope, "api", end, slices.Clone(evidence))
	}) {
		if err != nil {
			t.Fatal(err)
		}
	}
	var incidents, occurrences, rows, deliveries int
	for query, target := range map[string]*int{
		"SELECT count(*) FROM incidents WHERE project_id = $1":            &incidents,
		"SELECT count(*) FROM incident_occurrences WHERE project_id = $1": &occurrences,
		"SELECT count(*) FROM incident_evidence WHERE project_id = $1":    &rows,
		"SELECT count(*) FROM webhook_deliveries WHERE project_id = $1":   &deliveries,
	} {
		if err := database.pool.QueryRow(ctx, query, scope.ProjectID).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if incidents != 1 || occurrences != 1 || rows != 2 || deliveries != 1 {
		t.Fatalf("incidents=%d occurrences=%d evidence=%d deliveries=%d; replicas running the same scheduled window must not duplicate it", incidents, occurrences, rows, deliveries)
	}
}

func TestReplicasMaintainingPartitionsTogetherDoNotCollide(t *testing.T) {
	database, _ := newFlowTestDatabase(t)
	replicas := []*Database{database, openReplica(t), openReplica(t), openReplica(t)}
	for round := range 2 {
		for _, err := range runOnReplicas(replicas, func(_ int, replica *Database) error {
			return replica.EnsurePartitions(context.Background(), time.Now().UTC().AddDate(0, 0, 40+round), 3)
		}) {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
	var partitions int
	if err := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM pg_catalog.pg_inherits JOIN pg_catalog.pg_class AS parent ON parent.oid = inhparent WHERE parent.relname = 'events'").Scan(&partitions); err != nil {
		t.Fatal(err)
	}
	if partitions == 0 {
		t.Fatal("no event partitions exist after concurrent maintenance")
	}
}
