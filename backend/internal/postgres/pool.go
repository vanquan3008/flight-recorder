package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/migrations"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
)

type Database struct {
	pool *pgxpool.Pool
}

func Open(applicationContext context.Context, configuration config.DatabaseConfig) (*Database, error) {
	poolConfiguration, parseError := pgxpool.ParseConfig(configuration.URL)
	if parseError != nil {
		return nil, fmt.Errorf("parse database configuration: %w", parseError)
	}

	poolConfiguration.MaxConns = configuration.MaxConnections
	poolConfiguration.MinConns = configuration.MinConnections
	poolConfiguration.MaxConnLifetime = configuration.MaxConnectionLifetime
	poolConfiguration.MaxConnIdleTime = configuration.MaxConnectionIdleTime
	poolConfiguration.HealthCheckPeriod = configuration.HealthCheckPeriod

	databasePool, openError := pgxpool.NewWithConfig(applicationContext, poolConfiguration)
	if openError != nil {
		return nil, fmt.Errorf("open database pool: %w", openError)
	}
	return &Database{pool: databasePool}, nil
}

func (database *Database) Ready(readinessContext context.Context) error {
	if pingError := database.pool.Ping(readinessContext); pingError != nil {
		return fmt.Errorf("database ping failed: %w", pingError)
	}

	var version uint
	var dirty bool
	queryError := database.pool.QueryRow(readinessContext, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if queryError != nil {
		return fmt.Errorf("read schema version: %w", queryError)
	}
	if dirty || version != migrations.CurrentVersion {
		return fmt.Errorf("schema version is %d (dirty=%t), expected %d", version, dirty, migrations.CurrentVersion)
	}
	return nil
}

func (database *Database) Close() {
	database.pool.Close()
}

func (database *Database) EnsureIngestionScope(ctx context.Context, ingestion config.IngestionConfig) error {
	if _, executeError := database.pool.Exec(ctx, `
		WITH project AS (
			INSERT INTO projects (id, slug, name)
			VALUES ($1, $2, $3)
			ON CONFLICT (id) DO UPDATE
			SET slug = EXCLUDED.slug, name = EXCLUDED.name, updated_at = now()
			RETURNING id
		)
		INSERT INTO environments (id, project_id, name)
		SELECT $4, id, $5 FROM project
		ON CONFLICT (project_id, id) DO UPDATE SET name = EXCLUDED.name`,
		ingestion.ProjectID, ingestion.ProjectSlug, ingestion.ProjectName,
		ingestion.EnvironmentID, ingestion.EnvironmentName); executeError != nil {
		return fmt.Errorf("ensure ingestion scope: %w", executeError)
	}
	return nil
}

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
	if commitError := transaction.Commit(ctx); commitError != nil {
		return fmt.Errorf("commit event batch: %w", commitError)
	}
	return nil
}

func queueSignalRecord(batch *pgx.Batch, ingestion config.IngestionConfig, event telemetry.Event) (int, error) {
	switch event.Signal {
	case "":
		return 0, nil
	case telemetry.SignalTrace:
		resourceAttributes, resourceError := marshalJSONObject(event.ResourceAttributes)
		if resourceError != nil {
			return 0, fmt.Errorf("encode span resource attributes: %w", resourceError)
		}
		scopeAttributes, scopeError := marshalJSONObject(event.ScopeAttributes)
		if scopeError != nil {
			return 0, fmt.Errorf("encode span scope attributes: %w", scopeError)
		}
		recordAttributes, recordError := marshalJSONObject(event.RecordAttributes)
		if recordError != nil {
			return 0, fmt.Errorf("encode span attributes: %w", recordError)
		}
		spanEvents, eventsError := marshalJSONArray(event.SpanEvents)
		if eventsError != nil {
			return 0, fmt.Errorf("encode span events: %w", eventsError)
		}
		spanLinks, linksError := marshalJSONArray(event.SpanLinks)
		if linksError != nil {
			return 0, fmt.Errorf("encode span links: %w", linksError)
		}
		batch.Queue(`
			INSERT INTO spans (
				event_id, project_id, environment_id, trace_id, span_id, parent_span_id,
				trace_state, span_name, span_kind, started_at, ended_at, status_code, status_message, flags,
				service_name, service_version, resource_schema_url, resource_attributes,
				scope_name, scope_version, scope_schema_url, scope_attributes, span_attributes,
				span_events, span_links, dropped_attributes_count, dropped_events_count, dropped_links_count
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, $8, $9, $10, $11, $12, $13, $14,
				$15, $16, $17, $18,
				$19, $20, $21, $22, $23,
				$24, $25, $26, $27, $28
			)
			ON CONFLICT (event_id) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.TraceID, event.SpanID, nullableString(event.ParentSpanID),
			event.TraceState, event.Operation, event.SpanKind, event.Timestamp, event.Timestamp.Add(event.Duration), event.Status, event.StatusMessage, int64(event.TraceFlags),
			event.ServiceName, nullableString(event.ServiceVersion), event.ResourceSchemaURL, resourceAttributes,
			event.ScopeName, event.ScopeVersion, event.ScopeSchemaURL, scopeAttributes, recordAttributes,
			spanEvents, spanLinks, event.DroppedAttributes, event.DroppedEvents, event.DroppedLinks,
		)
		return 1, nil
	case telemetry.SignalLog:
		resourceAttributes, resourceError := marshalJSONObject(event.ResourceAttributes)
		if resourceError != nil {
			return 0, fmt.Errorf("encode log resource attributes: %w", resourceError)
		}
		scopeAttributes, scopeError := marshalJSONObject(event.ScopeAttributes)
		if scopeError != nil {
			return 0, fmt.Errorf("encode log scope attributes: %w", scopeError)
		}
		recordAttributes, recordError := marshalJSONObject(event.RecordAttributes)
		if recordError != nil {
			return 0, fmt.Errorf("encode log attributes: %w", recordError)
		}
		body, bodyError := marshalJSON(event.LogBody)
		if bodyError != nil {
			return 0, fmt.Errorf("encode log body: %w", bodyError)
		}
		batch.Queue(`
			INSERT INTO logs (
				event_id, project_id, environment_id, occurred_at, observed_at, trace_id, span_id,
				trace_flags, severity_number, severity_text, body, event_name, service_name, service_version,
				resource_schema_url, resource_attributes, scope_name, scope_version, scope_schema_url,
				scope_attributes, log_attributes, dropped_attributes_count
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, $11, $12, $13, $14,
				$15, $16, $17, $18, $19,
				$20, $21, $22
			)
			ON CONFLICT (event_id) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.Timestamp, nullableTime(event.ObservedTimestamp), nullableString(event.TraceID), nullableString(event.SpanID),
			int64(event.TraceFlags), event.SeverityNumber, event.Severity, body, event.LogEventName, event.ServiceName, nullableString(event.ServiceVersion),
			event.ResourceSchemaURL, resourceAttributes, event.ScopeName, event.ScopeVersion, event.ScopeSchemaURL,
			scopeAttributes, recordAttributes, event.DroppedAttributes,
		)
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported telemetry signal %q", event.Signal)
	}
}

func marshalJSON(value any) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(value)
}

func marshalJSONObject(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

func marshalJSONArray[T any](value []T) ([]byte, error) {
	if value == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(value)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableDuration(value time.Duration) any {
	if value == 0 {
		return nil
	}
	return value.Nanoseconds()
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
