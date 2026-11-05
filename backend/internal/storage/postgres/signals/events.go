package signals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/incidents"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgbatch"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// StoreEvents commits canonical events and their signal-specific records in one
// transaction. The ingestion pipeline acknowledges only after this returns.
// Errors caused by event content wrap telemetry.ErrInvalidEvent.
func (store *SignalStore) StoreEvents(ctx context.Context, ingestion config.IngestionConfig, events []telemetry.Event) error {
	return markInvalidEvents(store.storeEvents(ctx, ingestion, events))
}

func (store *SignalStore) storeEvents(ctx context.Context, ingestion config.IngestionConfig, events []telemetry.Event) error {
	if len(events) == 0 {
		return nil
	}
	services := make(map[string]struct{})
	for _, event := range events {
		if event.ProjectID != ingestion.ProjectID || event.Environment != ingestion.EnvironmentName {
			return errors.New("event ingestion scope mismatch")
		}
		services[event.ServiceName] = struct{}{}
	}

	// Rows are queued first and sent in one round trip; queued statements
	// report their errors from Send.
	queue := &pgbatch.Queue{}
	writes := sqlcstore.New(queue)
	serviceTimestamp := time.Now().UTC()
	for serviceName := range services {
		writes.EnsureService(ctx, sqlcstore.EnsureServiceParams{
			ID:        telemetry.DeterministicID(serviceTimestamp, ingestion.EnvironmentID, serviceName),
			ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID, Name: serviceName,
		})
	}
	for _, event := range events {
		if queueError := queueEvent(ctx, writes, ingestion, event); queueError != nil {
			return queueError
		}
	}

	return pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		if sendError := queue.Send(ctx, transaction); sendError != nil {
			return fmt.Errorf("insert event batch: %w", sendError)
		}
		startedAt := make([]time.Time, 0, len(events))
		for _, event := range events {
			if event.Signal == telemetry.SignalTrace {
				startedAt = append(startedAt, event.Timestamp)
			}
		}
		if len(startedAt) > 0 {
			if err := sqlcstore.New(transaction).MarkSpanRollupBuckets(ctx, sqlcstore.MarkSpanRollupBucketsParams{
				ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID, StartedAts: startedAt,
			}); err != nil {
				return fmt.Errorf("schedule span rollups: %w", err)
			}
		}
		return incidents.RecordOccurrences(ctx, transaction, ingestion, events)
	})
}

func queueEvent(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, event telemetry.Event) error {
	if err := queueEventEnvelope(ctx, writes, ingestion, event); err != nil {
		return err
	}
	return queueSignalRecord(ctx, writes, ingestion, event)
}

func queueEventEnvelope(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, event telemetry.Event) error {
	attributes, marshalError := json.Marshal(event.Attributes)
	if marshalError != nil {
		return fmt.Errorf("encode event attributes: %w", marshalError)
	}
	writes.InsertEvent(ctx, sqlcstore.InsertEventParams{
		ID: event.ID, ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		ServiceName: event.ServiceName, ServiceVersion: pgvalue.OrNil(event.ServiceVersion),
		TraceID: pgvalue.OrNil(event.TraceID), SpanID: pgvalue.OrNil(event.SpanID),
		ParentSpanID: pgvalue.OrNil(event.ParentSpanID), RequestID: pgvalue.OrNil(event.RequestID),
		EventType: string(event.Type), OccurredAt: event.Timestamp, DurationNS: pgvalue.NanosOrNil(event.Duration),
		Severity: pgvalue.OrNil(event.Severity), Status: pgvalue.OrNil(event.Status),
		HTTPStatus: pgvalue.OrNil(int32(event.HTTPStatus)), Operation: pgvalue.OrNil(event.Operation),
		ErrorType: pgvalue.OrNil(event.ErrorType), ErrorMessage: pgvalue.OrNil(event.ErrorMessage),
		Attributes: attributes,
	})
	return nil
}
