package main

import (
	"context"
	"errors"
	"strconv"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

func parsePostgresMigrationCommand(arguments []string) (string, int, error) {
	if len(arguments) == 1 {
		switch arguments[0] {
		case "up", "status", "version":
			return arguments[0], 0, nil
		}
	}
	if len(arguments) == 2 && arguments[0] == "down" {
		steps, err := strconv.Atoi(arguments[1])
		if err == nil && steps > 0 {
			return "down", steps, nil
		}
	}
	return "", 0, errors.New("PostgreSQL expects up, status, version, or down with a positive step count")
}

func runPostgresMigrations(arguments []string) int {
	operation, steps, err := parsePostgresMigrationCommand(arguments)
	if err != nil {
		return invalidMigrationArguments(err)
	}
	return runMigrationCommand("postgres", operation, func(ctx context.Context, cfg config.Config) (string, error) {
		return migrations.Run(ctx, cfg.Database.URL, operation, steps)
	})
}
