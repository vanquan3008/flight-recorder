package signals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/incidents"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgbatch"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// StoreLogIncidents retains only the error envelope needed by existing incident
// timelines and evidence. Raw bodies and OTLP attributes live in ClickHouse.
func (store *SignalStore) StoreLogIncidents(ctx context.Context, scope config.IngestionConfig, events []telemetry.Event) error {
	var failures []telemetry.Event
	queue := &pgbatch.Queue{}
	writes := sqlcstore.New(queue)
	for _, event := range events {
		if event.ProjectID != scope.ProjectID || event.Environment != scope.EnvironmentName || event.Signal != telemetry.SignalLog {
			return errors.New("log incident ingestion scope mismatch")
		}
		if !incident.Candidate(event) {
			continue
		}
		event.Attributes = map[string]any{}
		if err := queueEventEnvelope(ctx, writes, scope, event); err != nil {
			return err
		}
		failures = append(failures, event)
	}
	if len(failures) == 0 {
		return nil
	}
	return markInvalidEvents(pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		if err := queue.Send(ctx, tx); err != nil {
			return err
		}
		return incidents.RecordOccurrences(ctx, tx, scope, failures)
	}))
}
