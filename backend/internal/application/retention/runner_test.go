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
func (store *recordingStore) DeleteEventsBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	return store.record("telemetry", before)
}
func (store *recordingStore) CountEventsBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (store *recordingStore) DeleteMetricsBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	return store.record("metrics", before)
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
	pruneAll(context.Background(), logger, store, metrics.New("test", "test", "test"), policy.groups(store), policy, 10)
}

func TestEachGroupExpiresOnItsOwnCutoff(t *testing.T) {
	store := &recordingStore{cutoffs: map[string]time.Time{}}
	day := 24 * time.Hour
	started := time.Now()
	runOnce(store, Policy{Telemetry: 7 * day, Incidents: 90 * day, Audit: 180 * day, Deliveries: 30 * day})
	for group, age := range map[string]time.Duration{"telemetry": 7 * day, "metrics": 7 * day, "incidents": 90 * day, "audit": 180 * day, "deliveries": 30 * day} {
		cutoff, ran := store.cutoffs[group]
		if !ran {
			t.Fatalf("group %s did not run", group)
		}
		if drift := started.Add(-age).Sub(cutoff).Abs(); drift > time.Minute {
			t.Fatalf("group %s cutoff is %s away from now-%s", group, drift, age)
		}
	}
}

func TestZeroAgeKeepsAGroupForever(t *testing.T) {
	store := &recordingStore{cutoffs: map[string]time.Time{}}
	runOnce(store, Policy{Telemetry: time.Hour})
	if _, ran := store.cutoffs["incidents"]; ran {
		t.Fatal("a group with no max age was cleaned up")
	}
	if (Policy{}).Enabled() || !(Policy{Audit: time.Hour}).Enabled() {
		t.Fatal("Enabled must be true exactly when some group expires")
	}
}

func TestOneFailingGroupDoesNotStopTheOthers(t *testing.T) {
	store := &recordingStore{cutoffs: map[string]time.Time{}, failing: "telemetry"}
	runOnce(store, Policy{Telemetry: time.Hour, Incidents: time.Hour, Audit: time.Hour, Deliveries: time.Hour})
	for _, group := range []string{"metrics", "incidents", "audit", "deliveries"} {
		if _, ran := store.cutoffs[group]; !ran {
			t.Fatalf("group %s was skipped after telemetry failed", group)
		}
	}
}
