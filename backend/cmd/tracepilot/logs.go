package main

import (
	"context"
	"time"

	applicationlogs "github.com/vanquan3008/tracepilot/internal/application/logs"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

type logStore interface {
	StoreLogs(context.Context, string, string, []telemetry.Event) error
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLog(context.Context, string, string, string) (query.LogRecord, error)
	LogQuality(context.Context, string, string, string, time.Time, time.Time) (int64, int64, error)
	Ready(context.Context) error
	Close()
}

// PostgreSQL keeps all existing management, trace, metric and incident methods.
type storageBackend struct {
	*postgres.Database
	logs logStore
}

func buildStorageBackend(ctx context.Context, cfg config.Config, database *postgres.Database) (*storageBackend, error) {
	store, err := clickhouse.New(cfg.ClickHouse, cfg.Retention.MaxAge)
	if err != nil {
		return nil, err
	}
	if err := store.Ready(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return &storageBackend{Database: database, logs: store}, nil
}

func (backend *storageBackend) closeLogs() {
	backend.logs.Close()
}

func (backend *storageBackend) Ready(ctx context.Context) error {
	if err := backend.Database.Ready(ctx); err != nil {
		return err
	}
	return backend.logs.Ready(ctx)
}

func (backend *storageBackend) StoreEvents(ctx context.Context, scope config.IngestionConfig, events []telemetry.Event) error {
	return applicationlogs.Persist(ctx, events,
		func(ctx context.Context, logs []telemetry.Event) error {
			return backend.logs.StoreLogs(ctx, scope.ProjectID, scope.EnvironmentID, logs)
		},
		func(ctx context.Context, other []telemetry.Event) error {
			return backend.Database.StoreEvents(ctx, scope, other)
		},
		func(ctx context.Context, failures []telemetry.Event) error {
			return backend.Database.StoreLogIncidents(ctx, scope, failures)
		})
}

func (backend *storageBackend) ListLogs(ctx context.Context, project string, filter query.LogFilter) ([]query.LogRecord, error) {
	return backend.logs.ListLogs(ctx, project, filter)
}

func (backend *storageBackend) GetLog(ctx context.Context, project, environment, id string) (query.LogRecord, error) {
	return backend.logs.GetLog(ctx, project, environment, id)
}

func (backend *storageBackend) TelemetryQuality(ctx context.Context, project string, filter analysis.Filter) (analysis.Quality, error) {
	quality, err := backend.Database.TelemetryQuality(ctx, project, filter)
	if err != nil {
		return quality, err
	}
	quality.Logs, quality.UncorrelatedLogs, err = backend.logs.LogQuality(ctx, project, filter.EnvironmentID, filter.Service, filter.From, filter.To)
	return quality, err
}
