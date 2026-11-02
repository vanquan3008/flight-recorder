package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/analytics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/explorer"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/identity"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/incidents"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/maintenance"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/outbox"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/replays"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/signals"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/webhooks"
)

// scopeProvisioningLockKey serializes EnsureIngestionScope across replicas.
const scopeProvisioningLockKey = 7_203_411_002

// Database is the PostgreSQL adapter. It owns the pool and lifecycle; each
// embedded store owns one feature and shares the pool. Store methods are
// promoted, so callers depend on Database alone.
type Database struct {
	pool *pgxpool.Pool
	*signals.SignalStore
	*incidents.IncidentStore
	*outbox.OutboxStore
	*webhooks.WebhookStore
	*identity.IdentityStore
	*replays.ReplayStore
	*explorer.QueryStore
	*analytics.AnalyticsStore
	*maintenance.PartitionStore
}

// Open creates a database adapter without webhook-secret capabilities.
// Call OpenWithWebhookEncryptionKey when outbound notifications are enabled.
func Open(applicationContext context.Context, configuration config.DatabaseConfig) (*Database, error) {
	return open(applicationContext, configuration, "")
}

// OpenWithWebhookEncryptionKey creates a database adapter able to encrypt and
// decrypt webhook secrets. The bootstrap layer supplies this secret explicitly
// so loading configuration never requires a post-validation mutation.
func OpenWithWebhookEncryptionKey(applicationContext context.Context, configuration config.DatabaseConfig, webhookEncryptionKey string) (*Database, error) {
	return open(applicationContext, configuration, webhookEncryptionKey)
}

func open(applicationContext context.Context, configuration config.DatabaseConfig, webhookEncryptionKey string) (*Database, error) {
	poolConfiguration, parseError := pgxpool.ParseConfig(configuration.URL)
	if parseError != nil {
		return nil, fmt.Errorf("parse database configuration: %w", parseError)
	}

	poolConfiguration.MaxConns = configuration.MaxConnections
	poolConfiguration.MinConns = configuration.MinConnections
	poolConfiguration.MaxConnLifetime = configuration.MaxConnectionLifetime
	poolConfiguration.MaxConnIdleTime = configuration.MaxConnectionIdleTime
	poolConfiguration.HealthCheckPeriod = configuration.HealthCheckPeriod
	applySessionParameters(poolConfiguration.ConnConfig, configuration)

	databasePool, openError := pgxpool.NewWithConfig(applicationContext, poolConfiguration)
	if openError != nil {
		return nil, fmt.Errorf("open database pool: %w", openError)
	}
	var webhookKey []byte
	if webhookEncryptionKey != "" {
		var keyError error
		webhookKey, keyError = notificationdomain.DecodeEncryptionKey(webhookEncryptionKey)
		if keyError != nil {
			databasePool.Close()
			return nil, fmt.Errorf("parse webhook encryption key: %w", keyError)
		}
	}
	return &Database{
		pool:           databasePool,
		SignalStore:    signals.New(databasePool),
		IncidentStore:  incidents.New(databasePool),
		OutboxStore:    outbox.New(databasePool, webhookKey),
		WebhookStore:   webhooks.New(databasePool, webhookKey),
		IdentityStore:  identity.New(databasePool),
		ReplayStore:    replays.New(databasePool),
		QueryStore:     explorer.New(databasePool),
		AnalyticsStore: analytics.New(databasePool),
		PartitionStore: maintenance.New(databasePool),
	}, nil
}

func (database *Database) Ready(readinessContext context.Context) error {
	if pingError := database.pool.Ping(readinessContext); pingError != nil {
		return fmt.Errorf("database ping failed: %w", pingError)
	}

	state, queryError := sqlcstore.New(database.pool).GetSchemaVersion(readinessContext)
	if queryError != nil {
		return fmt.Errorf("read schema version: %w", queryError)
	}
	if state.Dirty || uint(state.Version) != migrations.CurrentVersion {
		return fmt.Errorf("schema version is %d (dirty=%t), expected %d", state.Version, state.Dirty, migrations.CurrentVersion)
	}
	return nil
}

// PoolSnapshot reports how busy the connection pool is, for saturation metrics.
func (database *Database) PoolSnapshot() metrics.PoolSnapshot {
	stats := database.pool.Stat()
	return metrics.PoolSnapshot{
		Acquired: stats.AcquiredConns(), Idle: stats.IdleConns(), Max: stats.MaxConns(),
		EmptyAcquires: stats.EmptyAcquireCount(), AcquireWait: stats.AcquireDuration(),
	}
}

func (database *Database) Close() {
	database.pool.Close()
}

func (database *Database) EnsureIngestionScope(ctx context.Context, ingestion config.IngestionConfig) error {
	// Replicas started together all provision the same project. Its rows have
	// several unique keys, so concurrent upserts can collide on one the conflict
	// target does not name; the lock lets them run one after another instead.
	if executeError := pgx.BeginFunc(ctx, database.pool, func(transaction pgx.Tx) error {
		queries := sqlcstore.New(transaction)
		if lockError := queries.AcquireTransactionLock(ctx, scopeProvisioningLockKey); lockError != nil {
			return lockError
		}
		return queries.EnsureIngestionScope(ctx, sqlcstore.EnsureIngestionScopeParams{
			ProjectID: ingestion.ProjectID, ProjectSlug: ingestion.ProjectSlug, ProjectName: ingestion.ProjectName,
			EnvironmentID: ingestion.EnvironmentID, EnvironmentName: ingestion.EnvironmentName,
		})
	}); executeError != nil {
		return fmt.Errorf("ensure ingestion scope: %w", executeError)
	}
	return nil
}
