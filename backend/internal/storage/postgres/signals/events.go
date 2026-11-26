package signals

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/incidents"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgbatch"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// StoreEvents records the failing spans and logs of a batch, a compact envelope
// without attributes and the incident occurrences they open, in one
// transaction. Other events are skipped: their telemetry lives in ClickHouse.
// The ingestion pipeline acknowledges only after this returns. Errors caused by
// event content wrap telemetry.ErrInvalidEvent.
func (store *SignalStore) StoreEvents(ctx context.Context, ingestion config.IngestionConfig, events []telemetry.Event) error {
	var failures []telemetry.Event
	for _, event := range events {
		if event.ProjectID != ingestion.ProjectID || event.Environment != ingestion.EnvironmentName {
			return errors.New("event ingestion scope mismatch")
		}
		if !incident.Candidate(event) {
			continue
		}
		event.Attributes = map[string]any{}
		failures = append(failures, event)
	}
	if len(failures) == 0 {
		return nil
	}
	// Replicas can receive the same batch in different orders. Acquire row locks in primary key order.
	slices.SortStableFunc(failures, func(left, right telemetry.Event) int {
		return cmp.Or(cmp.Compare(left.ID, right.ID), left.Timestamp.Compare(right.Timestamp))
	})
	queue := &pgbatch.Queue{}
	writes := sqlcstore.New(queue)
	for _, event := range failures {
		if err := queueEventEnvelope(ctx, writes, ingestion, event); err != nil {
			return err
		}
	}
	return markInvalidEvents(pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		if err := queue.Send(ctx, transaction); err != nil {
			return fmt.Errorf("insert event batch: %w", err)
		}
		return incidents.RecordOccurrences(ctx, transaction, ingestion, failures)
	}))
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
