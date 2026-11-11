package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

func runAllMigrations(arguments []string) int {
	if len(arguments) != 1 || arguments[0] != "all" {
		return invalidMigrationArguments(errors.New("migrate all takes no extra arguments"))
	}
	return runMigrationCommand("all", "all", func(ctx context.Context, cfg config.Config) (string, error) {
		if err := migrateAll(ctx, cfg); err != nil {
			return "", err
		}
		return "both database schemas are ready", nil
	})
}

// migrateAll is an explicit batch command; it owns no shared version or journal.
func migrateAll(ctx context.Context, cfg config.Config) error {
	if _, err := migrations.Run(ctx, cfg.Database.URL, "up", 0); err != nil {
		return fmt.Errorf("PostgreSQL migration: %w", err)
	}
	if _, err := migrateClickHouse(ctx, cfg.ClickHouse, cfg.Retention.MaxAge, "up"); err != nil {
		return fmt.Errorf("ClickHouse migration failed after PostgreSQL completed; PostgreSQL was not rolled back: %w", err)
	}
	backend, err := connectStorageBackend(ctx, cfg, "")
	if err != nil {
		return err
	}
	backend.Close()
	return nil
}
