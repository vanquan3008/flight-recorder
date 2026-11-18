package main

import (
	"context"
	"errors"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
)

func parseClickHouseMigrationCommand(arguments []string) (string, error) {
	if len(arguments) == 1 {
		switch arguments[0] {
		case migrationUp, migrationStatus:
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
		return clickhouse.RunMigration(ctx, cfg.ClickHouse, cfg.Retention.MaxAge, operation)
	})
}
