package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
)

func parseClickHouseMigrationCommand(arguments []string) (string, error) {
	if len(arguments) == 1 {
		switch arguments[0] {
		case "up", "status":
			return arguments[0], nil
		}
	}
	return "", errors.New("ClickHouse expects up or status")
}

func runClickHouseMigrations(arguments []string) int {
	operation, err := parseClickHouseMigrationCommand(arguments)
	if err != nil {
		return invalidMigrationArguments(err)
	}
	return runMigrationCommand("clickhouse", operation, func(ctx context.Context, cfg config.Config) (string, error) {
		return migrateClickHouse(ctx, cfg.ClickHouse, cfg.Retention.MaxAge, operation)
	})
}

func migrateClickHouse(ctx context.Context, cfg config.ClickHouseConfig, maxAge time.Duration, operation string) (string, error) {
	if operation != "up" && operation != "status" {
		return "", fmt.Errorf("unsupported ClickHouse migration operation %q", operation)
	}
	store, err := clickhouse.New(cfg, maxAge)
	if err != nil {
		return "", err
	}
	defer store.Close()
	if operation == "status" {
		state, err := store.MigrationStatus(ctx)
		return state.String(), err
	}
	if err := store.Migrate(ctx); err != nil {
		return "", err
	}
	return "migrations applied", nil
}
