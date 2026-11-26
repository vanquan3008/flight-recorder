package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vanquan3008/tracepilot/internal/application/lifecycle"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

type databaseConnections struct {
	metadata  *postgres.Database
	telemetry *clickhouse.Store
	lifecycle *lifecycle.Databases
}

func openDatabases(ctx context.Context, cfg config.Config) (*databaseConnections, error) {
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
	return connectDatabases(ctx, cfg, cfg.Notifications.EncryptionKey)
}

func connectDatabases(ctx context.Context, cfg config.Config, webhookEncryptionKey string) (*databaseConnections, error) {
	ctx, cancel := context.WithTimeout(ctx, max(cfg.Database.ConnectTimeout, cfg.ClickHouse.RequestTimeout))
	defer cancel()
	database, err := postgres.OpenWithWebhookEncryptionKey(ctx, cfg.Database, webhookEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	telemetry, err := clickhouse.New(cfg.ClickHouse)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize ClickHouse: %w", err)
	}
	connections := &databaseConnections{metadata: database, telemetry: telemetry, lifecycle: lifecycle.NewDatabases(database, telemetry)}
	if err := connections.lifecycle.Ready(ctx); err != nil {
		connections.lifecycle.Close()
		return nil, err
	}
	return connections, nil
}

func migrateAll(ctx context.Context, cfg config.Config) error {
	if _, err := migrations.RunWithConfig(ctx, cfg.Database, "up", 0); err != nil {
		return fmt.Errorf("PostgreSQL migration: %w", err)
	}
	if _, err := clickhouse.RunMigration(ctx, cfg.ClickHouse, "up"); err != nil {
		return fmt.Errorf("ClickHouse migration failed after PostgreSQL completed; PostgreSQL was not rolled back: %w", err)
	}
	connections, err := connectDatabases(ctx, cfg, "")
	if err != nil {
		return err
	}
	connections.lifecycle.Close()
	return nil
}
