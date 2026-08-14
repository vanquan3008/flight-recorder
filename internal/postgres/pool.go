package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/flight-observability/flight/internal/config"
	"github.com/flight-observability/flight/internal/migrations"
	"github.com/flight-observability/flight/internal/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func (database *Database) StoreEvents(ctx context.Context, ingestion config.IngestionConfig, events []telemetry.Event) error {
	if len(events) == 0 {
		return nil
	}
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return fmt.Errorf("begin event batch: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO projects (id, slug, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET slug = EXCLUDED.slug, name = EXCLUDED.name, updated_at = now()`,
		ingestion.ProjectID, ingestion.ProjectSlug, ingestion.ProjectName); insertError != nil {
		return fmt.Errorf("ensure ingestion project: %w", insertError)
	}
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO environments (id, project_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, id) DO UPDATE SET name = EXCLUDED.name`,
		ingestion.EnvironmentID, ingestion.ProjectID, ingestion.EnvironmentName); insertError != nil {
		return fmt.Errorf("ensure ingestion environment: %w", insertError)
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
		serviceID := telemetry.DeterministicID(time.Now().UTC(), ingestion.EnvironmentID, event.ServiceName)
		if _, insertError := transaction.Exec(ctx, `
			INSERT INTO services (id, project_id, environment_id, name)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id, environment_id, name) DO NOTHING`,
			serviceID, ingestion.ProjectID, ingestion.EnvironmentID, event.ServiceName); insertError != nil {
			return fmt.Errorf("ensure ingestion service: %w", insertError)
		}
	}

	batch := &pgx.Batch{}
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
				operation, error_type, error_message, attributes, payload_ref
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15,
				$16, $17, $18, $19, $20
			)
			ON CONFLICT (id) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.ServiceName, nullableString(event.ServiceVersion),
			nullableString(event.TraceID), nullableString(event.SpanID), nullableString(event.ParentSpanID), nullableString(event.RequestID), string(event.Type),
			event.Timestamp, nullableDuration(event.Duration), nullableString(event.Severity), nullableString(event.Status), nullableInt(event.HTTPStatus),
			nullableString(event.Operation), nullableString(event.ErrorType), nullableString(event.ErrorMessage), attributes, nullableString(event.PayloadRef),
		)
	}
	results := transaction.SendBatch(ctx, batch)
	for range events {
		if _, executeError := results.Exec(); executeError != nil {
			_ = results.Close()
			return fmt.Errorf("insert event batch: %w", executeError)
		}
	}
	if closeError := results.Close(); closeError != nil {
		return fmt.Errorf("close event batch: %w", closeError)
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return fmt.Errorf("commit event batch: %w", commitError)
	}
	return nil
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
