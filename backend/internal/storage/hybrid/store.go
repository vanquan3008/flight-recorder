// Package hybrid combines the PostgreSQL store with the ClickHouse log store.
// PostgreSQL keeps management, trace, metric and incident data; ClickHouse keeps raw logs.
// The two adapters must not import each other, so their composition lives here.
package hybrid

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"golang.org/x/sync/errgroup"
)

type logStore interface {
	StoreLogs(context.Context, string, string, []telemetry.Event) error
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLogSince(context.Context, string, string, string, time.Time) (query.LogRecord, error)
	LogQuality(context.Context, string, string, string, time.Time, time.Time) (int64, int64, error)
	ExpireProjectLogs(context.Context, string, time.Time) error
	Ready(context.Context) error
	Close()
}

// Store exposes every PostgreSQL method plus the log reads served by ClickHouse.
// Its promoted StoreEvents writes PostgreSQL only; ingestion routes logs through
// the application log workflow before calling it.
type Store struct {
	*postgres.Database
	logs                logStore
	defaultRetentionAge time.Duration
}

func (store *Store) Close() {
	store.logs.Close()
	store.Database.Close()
}

// Ready checks both databases concurrently; the first failure cancels the other probe.
func (store *Store) Ready(ctx context.Context) error {
	return checkDatabases(ctx, store.Database, store.logs)
}

func (store *Store) StoreLogs(ctx context.Context, project, environment string, logs []telemetry.Event) error {
	return store.logs.StoreLogs(ctx, project, environment, logs)
}

func (store *Store) ListLogs(ctx context.Context, project string, filter query.LogFilter) ([]query.LogRecord, error) {
	cutoff, err := store.logRetentionCutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	if cutoff.After(filter.From) {
		filter.From = cutoff
	}
	return store.logs.ListLogs(ctx, project, filter)
}

func (store *Store) GetLog(ctx context.Context, project, environment, id string) (query.LogRecord, error) {
	cutoff, err := store.logRetentionCutoff(ctx, project)
	if err != nil {
		return query.LogRecord{}, err
	}
	return store.logs.GetLogSince(ctx, project, environment, id, cutoff)
}

func (store *Store) TelemetryQuality(ctx context.Context, project string, filter analysis.Filter) (analysis.Quality, error) {
	quality, err := store.Database.TelemetryQuality(ctx, project, filter)
	if err != nil {
		return quality, err
	}
	cutoff, err := store.logRetentionCutoff(ctx, project)
	if err != nil {
		return quality, err
	}
	if cutoff.After(filter.From) {
		filter.From = cutoff
	}
	quality.Logs, quality.UncorrelatedLogs, err = store.logs.LogQuality(ctx, project, filter.EnvironmentID, filter.Service, filter.From, filter.To)
	return quality, err
}

func (store *Store) logRetentionCutoff(ctx context.Context, project string) (time.Time, error) {
	days, err := store.GetProjectRetention(ctx, project)
	if err != nil {
		return time.Time{}, err
	}
	if age := telemetry.RetentionAge(days, store.defaultRetentionAge); age > 0 {
		cutoff := time.Now().UTC().Add(-age)
		if cutoff.After(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
			return cutoff, nil
		}
	}
	return time.Time{}, nil
}

func (store *Store) PruneProjectLogs(ctx context.Context, projectID string, now time.Time, fallback time.Duration) error {
	return store.WithProjectRetention(ctx, projectID, now, fallback, func(before time.Time) error {
		return store.logs.ExpireProjectLogs(ctx, projectID, before)
	})
}

type databaseProbe interface {
	Ready(context.Context) error
}

func checkDatabases(ctx context.Context, database, logs databaseProbe) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := database.Ready(ctx); err != nil {
			return fmt.Errorf("PostgreSQL readiness: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		if err := logs.Ready(ctx); err != nil {
			return fmt.Errorf("ClickHouse readiness: %w", err)
		}
		return nil
	})
	return group.Wait()
}
