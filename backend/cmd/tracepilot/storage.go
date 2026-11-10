package main

import (
	"context"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"golang.org/x/sync/errgroup"
)

// openStorageBackend owns both clients and closes them if startup fails.
func openStorageBackend(ctx context.Context, cfg config.Config) (*storageBackend, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, max(cfg.Database.ConnectTimeout, cfg.ClickHouse.RequestTimeout))
	defer cancel()
	database, err := postgres.OpenWithWebhookEncryptionKey(ctx, cfg.Database, cfg.Notifications.EncryptionKey)
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
