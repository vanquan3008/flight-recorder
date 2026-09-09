package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// StoreEvents commits canonical events and their signal-specific records in one
// transaction. The ingestion pipeline acknowledges only after this returns.
func (database *Database) StoreEvents(ctx context.Context, ingestion config.IngestionConfig, events []telemetry.Event) error {
	if len(events) == 0 {
		return nil
	}

	services := make(map[string]struct{})
	for _, event := range events {
		if event.ProjectID != ingestion.ProjectID || event.Environment != ingestion.EnvironmentName {
			return errors.New("event ingestion scope mismatch")
		}
		if _, exists := services[event.ServiceName]; exists {
			continue
		}
		services[event.ServiceName] = struct{}{}
	}

	batch := &pgx.Batch{}
	statementCount := 0
	serviceTimestamp := time.Now().UTC()
	for serviceName := range services {
		serviceID := telemetry.DeterministicID(serviceTimestamp, ingestion.EnvironmentID, serviceName)
		batch.Queue(`
			INSERT INTO services (id, project_id, environment_id, name)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id, environment_id, name) DO NOTHING`,
			serviceID, ingestion.ProjectID, ingestion.EnvironmentID, serviceName)
		statementCount++
	}

	for _, event := range events {
		attributes, marshalError := json.Marshal(event.Attributes)
		if marshalError != nil {
			return fmt.Errorf("encode event attributes: %w", marshalError)
		}
		batch.Queue(`
			INSERT INTO events (
				id, project_id, environment_id, service_name, service_version,
				trace_id, span_id, parent_span_id, request_id, event_type,
				occurred_at, duration_ns, severity, status, http_status,
				operation, error_type, error_message, attributes
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15,
				$16, $17, $18, $19
			)
			ON CONFLICT (id) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.ServiceName, nullableString(event.ServiceVersion),
			nullableString(event.TraceID), nullableString(event.SpanID), nullableString(event.ParentSpanID), nullableString(event.RequestID), string(event.Type),
			event.Timestamp, nullableDuration(event.Duration), nullableString(event.Severity), nullableString(event.Status), nullableInt(event.HTTPStatus),
			nullableString(event.Operation), nullableString(event.ErrorType), nullableString(event.ErrorMessage), attributes,
		)
		statementCount++
		signalStatements, signalError := queueSignalRecord(batch, ingestion, event)
		if signalError != nil {
			return signalError
		}
		statementCount += signalStatements
	}

	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return fmt.Errorf("begin event batch: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	results := transaction.SendBatch(ctx, batch)
	for range statementCount {
		if _, executeError := results.Exec(); executeError != nil {
			_ = results.Close()
			return fmt.Errorf("insert event batch: %w", executeError)
		}
	}
	if closeError := results.Close(); closeError != nil {
		return fmt.Errorf("close event batch: %w", closeError)
	}
	if incidentError := storeIncidents(ctx, transaction, ingestion, events); incidentError != nil {
		return incidentError
	}
	if securityError := storeSecurityEvents(ctx, transaction, ingestion, events); securityError != nil {
		return securityError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return fmt.Errorf("commit event batch: %w", commitError)
	}
	return nil
}
