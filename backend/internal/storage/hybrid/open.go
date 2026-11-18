package hybrid

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

// Open applies each database's pending migrations when its auto-migration flag is set,
// then connects and verifies both schemas. A failure leaves no client open.
func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Database.AutoMigrate {
		slog.Info("automatic PostgreSQL migration enabled")
		if _, err := migrations.RunWithConfig(ctx, cfg.Database, "up", 0); err != nil {
			return nil, fmt.Errorf("automatic PostgreSQL migration: %w", err)
		}
	}
	if cfg.ClickHouse.AutoMigrate {
		slog.Info("automatic ClickHouse migration enabled")
		if _, err := clickhouse.RunMigration(ctx, cfg.ClickHouse, "up"); err != nil {
			return nil, fmt.Errorf("automatic ClickHouse migration: %w", err)
		}
	}
	return connect(ctx, cfg, cfg.Notifications.EncryptionKey)
}

// connect checks both schemas without performing migrations.
func connect(ctx context.Context, cfg config.Config, webhookEncryptionKey string) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, max(cfg.Database.ConnectTimeout, cfg.ClickHouse.RequestTimeout))
	defer cancel()
	database, err := postgres.OpenWithWebhookEncryptionKey(ctx, cfg.Database, webhookEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	logs, err := clickhouse.New(cfg.ClickHouse)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize ClickHouse: %w", err)
	}
	store := &Store{Database: database, logs: logs, defaultRetentionAge: cfg.Retention.MaxAge}
	if err := store.Ready(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

// MigrateAll upgrades PostgreSQL, then ClickHouse, then checks both schemas. The two
// databases share no version, journal or transaction: it stops at the first failure
// and never rolls back a completed step.
func MigrateAll(ctx context.Context, cfg config.Config) error {
	if _, err := migrations.RunWithConfig(ctx, cfg.Database, "up", 0); err != nil {
		return fmt.Errorf("PostgreSQL migration: %w", err)
	}
	if _, err := clickhouse.RunMigration(ctx, cfg.ClickHouse, "up"); err != nil {
		return fmt.Errorf("ClickHouse migration failed after PostgreSQL completed; PostgreSQL was not rolled back: %w", err)
	}
	// Readiness needs only database credentials, never application secrets.
	store, err := connect(ctx, cfg, "")
	if err != nil {
		return err
	}
	store.Close()
	return nil
}
