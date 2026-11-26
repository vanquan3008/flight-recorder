package main

import (
	"context"

	applicationlogs "github.com/vanquan3008/tracepilot/internal/application/logs"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

type eventTelemetry interface {
	StoreLogs(context.Context, string, string, []telemetry.Event) error
	StoreSpans(context.Context, string, string, []telemetry.Event) error
}

type incidentEventWriter interface {
	StoreEvents(context.Context, config.IngestionConfig, []telemetry.Event) error
}

// eventWriter stores logs and spans in ClickHouse and their failures, which
// open incidents, in PostgreSQL.
type eventWriter struct {
	telemetry eventTelemetry
	incidents incidentEventWriter
}

func (writer eventWriter) StoreEvents(ctx context.Context, scope config.IngestionConfig, events []telemetry.Event) error {
	return applicationlogs.Persist(ctx, events,
		func(ctx context.Context, logs []telemetry.Event) error {
			return writer.telemetry.StoreLogs(ctx, scope.ProjectID, scope.EnvironmentID, logs)
		},
		func(ctx context.Context, spans []telemetry.Event) error {
			return writer.telemetry.StoreSpans(ctx, scope.ProjectID, scope.EnvironmentID, spans)
		},
		func(ctx context.Context, failures []telemetry.Event) error {
			return writer.incidents.StoreEvents(ctx, scope, failures)
		})
}
