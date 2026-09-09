package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// Database owns the PostgreSQL pool and the generated query adapter. Feature
// files in this package keep persistence operations separate from lifecycle.
type Database struct {
	pool                 *pgxpool.Pool
	queries              *sqlcstore.Queries
	webhookEncryptionKey []byte
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
	var webhookKey []byte
	if configuration.WebhookEncryptionKey != "" {
		var keyError error
		webhookKey, keyError = notificationdomain.DecodeEncryptionKey(configuration.WebhookEncryptionKey)
		if keyError != nil {
			databasePool.Close()
			return nil, fmt.Errorf("parse webhook encryption key: %w", keyError)
		}
	}
	return &Database{pool: databasePool, queries: sqlcstore.New(databasePool), webhookEncryptionKey: webhookKey}, nil
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
