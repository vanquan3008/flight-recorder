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
	GetLog(context.Context, string, string, string) (query.LogRecord, error)
	LogQuality(context.Context, string, string, string, time.Time, time.Time) (int64, int64, error)
	Ready(context.Context) error
	Close()
}

// Store exposes every PostgreSQL method plus the log reads served by ClickHouse.
// Its promoted StoreEvents writes PostgreSQL only; ingestion routes logs through
// the application log workflow before calling it.
type Store struct {
	*postgres.Database
	logs logStore
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
	return store.logs.ListLogs(ctx, project, filter)
}

func (store *Store) GetLog(ctx context.Context, project, environment, id string) (query.LogRecord, error) {
	return store.logs.GetLog(ctx, project, environment, id)
}

func (store *Store) TelemetryQuality(ctx context.Context, project string, filter analysis.Filter) (analysis.Quality, error) {
	quality, err := store.Database.TelemetryQuality(ctx, project, filter)
	if err != nil {
		return quality, err
	}
	quality.Logs, quality.UncorrelatedLogs, err = store.logs.LogQuality(ctx, project, filter.EnvironmentID, filter.Service, filter.From, filter.To)
	return quality, err
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
