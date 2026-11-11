package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
	"golang.org/x/sync/errgroup"
)

// openStorageBackend owns both clients and closes them if startup fails.
func openStorageBackend(ctx context.Context, cfg config.Config) (*storageBackend, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Database.AutoMigrate {
		slog.Info("automatic PostgreSQL migration enabled")
		if _, err := migrations.Run(ctx, cfg.Database.URL, "up", 0); err != nil {
			return nil, fmt.Errorf("automatic PostgreSQL migration: %w", err)
		}
	}
	if cfg.ClickHouse.AutoMigrate {
		slog.Info("automatic ClickHouse migration enabled")
		if _, err := migrateClickHouse(ctx, cfg.ClickHouse, cfg.Retention.MaxAge, "up"); err != nil {
			return nil, fmt.Errorf("automatic ClickHouse migration: %w", err)
		}
	}
	return connectStorageBackend(ctx, cfg, cfg.Notifications.EncryptionKey)
}

// connectStorageBackend checks both schemas without performing migrations.
func connectStorageBackend(ctx context.Context, cfg config.Config, webhookEncryptionKey string) (*storageBackend, error) {
	ctx, cancel := context.WithTimeout(ctx, max(cfg.Database.ConnectTimeout, cfg.ClickHouse.RequestTimeout))
	defer cancel()
	database, err := postgres.OpenWithWebhookEncryptionKey(ctx, cfg.Database, webhookEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	logs, err := clickhouse.New(cfg.ClickHouse, cfg.Retention.MaxAge)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize ClickHouse: %w", err)
	}
	backend := &storageBackend{Database: database, logs: logs}
	if err := backend.Ready(ctx); err != nil {
		backend.Close()
		return nil, err
	}
	return backend, nil
}

func checkDatabases(ctx context.Context, database, logs databaseProbe) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := database.Ready(ctx); err != nil {
			return fmt.Errorf("PostgreSQL readiness: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		if err := logs.Ready(ctx); err != nil {
			return fmt.Errorf("ClickHouse readiness: %w", err)
		}
		return nil
	})
	return group.Wait()
}
