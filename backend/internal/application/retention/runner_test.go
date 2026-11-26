package retention

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestDeleteExpiredDrainsBacklog(t *testing.T) {
	calls := 0
	deleted, pruneError := DeleteExpired(context.Background(), time.Now(), 2, func(context.Context, time.Time, int) (int64, error) {
		calls++
		if calls < 3 {
			return 2, nil
		}
		return 1, nil
	})
	if pruneError != nil || deleted != 5 || calls != 3 {
		t.Fatalf("deleted=%d calls=%d error=%v", deleted, calls, pruneError)
	}
}

func TestDeleteExpiredStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	deleted, pruneError := DeleteExpired(ctx, time.Now(), 1, func(context.Context, time.Time, int) (int64, error) {
		cancel()
		return 1, nil
	})
	if !errors.Is(pruneError, context.Canceled) || deleted != 1 {
		t.Fatalf("deleted=%d error=%v", deleted, pruneError)
	}
}

func TestDeleteExpiredReturnsDeleteError(t *testing.T) {
	wantError := errors.New("database unavailable")
	deleted, pruneError := DeleteExpired(context.Background(), time.Now(), 10, func(context.Context, time.Time, int) (int64, error) {
		return 3, wantError
	})
	if !errors.Is(pruneError, wantError) || deleted != 3 {
		t.Fatalf("deleted=%d error=%v", deleted, pruneError)
	}
}

type recordingStore struct {
	cutoffs map[string]time.Time
	failing string
}

func (store *recordingStore) record(group string, before time.Time) (int64, error) {
	store.cutoffs[group] = before
	if group == store.failing {
		return 0, errors.New("database unavailable")
	}
	return 0, nil
}
func (store *recordingStore) ListRetentionProjects(context.Context) ([]string, error) {
	return []string{"alpha", "beta"}, nil
}
func (store *recordingStore) CountExpiredProjectEvents(context.Context, time.Time, time.Duration) (int64, error) {
	return 0, nil
}
func (store *recordingStore) DropExpiredTelemetryPartitions(_ context.Context, now time.Time, fallback time.Duration) (int64, error) {
	return store.record("shared-partitions", now.Add(-fallback))
}
func (store *recordingStore) DropExpiredPartitions(_ context.Context, now time.Time, fallback time.Duration) error {
	_, err := store.record("clickhouse-partitions", now.Add(-fallback))
	return err
}
func (store *recordingStore) PruneProjectTelemetry(_ context.Context, project string, now time.Time, fallback time.Duration, _ int) (int64, error) {
	return store.record("telemetry-"+project, now.Add(-fallback))
}
func (store *recordingStore) PruneProject(_ context.Context, project string, now time.Time, fallback time.Duration) error {
	_, err := store.record("logs-"+project, now.Add(-fallback))
	return err
}
func (store *recordingStore) DeleteIncidentsBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	return store.record("incidents", before)
}
func (store *recordingStore) DeleteAuditBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	return store.record("audit", before)
}
func (store *recordingStore) DeleteDeliveriesBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	return store.record("deliveries", before)
}

func runOnce(store *recordingStore, policy Policy) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pruneAll(context.Background(), logger, store, store, metrics.New("test", "test", "test"), policy, 10, testNow)
}

func TestEachGroupExpiresOnItsOwnCutoff(t *testing.T) {
	store := &recordingStore{cutoffs: map[string]time.Time{}}
	day := 24 * time.Hour
	runOnce(store, Policy{Telemetry: 7 * day, Incidents: 90 * day, Audit: 180 * day, Deliveries: 30 * day})
	for group, age := range map[string]time.Duration{"telemetry-alpha": 7 * day, "logs-beta": 7 * day, "incidents": 90 * day, "audit": 180 * day, "deliveries": 30 * day} {
		cutoff, ran := store.cutoffs[group]
		if !ran {
			t.Fatalf("group %s did not run", group)
		}
		if want := testNow.Add(-age); !cutoff.Equal(want) {
			t.Fatalf("group %s cutoff = %v, want %v", group, cutoff, want)
		}
	}
}

func TestZeroAgeKeepsAGroupForever(t *testing.T) {
	store := &recordingStore{cutoffs: map[string]time.Time{}}
	runOnce(store, Policy{})
	if _, ran := store.cutoffs["incidents"]; ran {
		t.Fatal("a group with no max age was cleaned up")
	}
	if _, ran := store.cutoffs["telemetry-alpha"]; !ran {
		t.Fatal("the worker skipped project overrides with an indefinite default")
	}
}

func TestOneFailingGroupDoesNotStopTheOthers(t *testing.T) {
	store := &recordingStore{cutoffs: map[string]time.Time{}, failing: "telemetry-alpha"}
	runOnce(store, Policy{Telemetry: time.Hour, Incidents: time.Hour, Audit: time.Hour, Deliveries: time.Hour})
	for _, group := range []string{"logs-alpha", "telemetry-beta", "logs-beta", "incidents", "audit", "deliveries"} {
		if _, ran := store.cutoffs[group]; !ran {
			t.Fatalf("group %s was skipped after telemetry failed", group)
		}
	}
}
