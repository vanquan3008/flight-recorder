package main

import (
	"context"

	applicationlogs "github.com/vanquan3008/tracepilot/internal/application/logs"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/hybrid"
)

// eventWriter stores logs and spans in ClickHouse and their failures, which
// open incidents, in PostgreSQL.
type eventWriter struct {
	store *hybrid.Store
}

func (writer eventWriter) StoreEvents(ctx context.Context, scope config.IngestionConfig, events []telemetry.Event) error {
	store := writer.store
	return applicationlogs.Persist(ctx, events,
		func(ctx context.Context, logs []telemetry.Event) error {
			return store.StoreLogs(ctx, scope.ProjectID, scope.EnvironmentID, logs)
		},
		func(ctx context.Context, spans []telemetry.Event) error {
			return store.StoreSpans(ctx, scope.ProjectID, scope.EnvironmentID, spans)
		},
		func(ctx context.Context, failures []telemetry.Event) error {
			return store.Database.StoreEvents(ctx, scope, failures)
		})
}
