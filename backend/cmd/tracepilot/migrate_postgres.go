package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

var errInvalidPostgresMigration = errors.New(
	"expected: up, status, version, or down <positive-steps>",
)

func parsePostgresMigrationCommand(args []string) (string, int, error) {
	if len(args) == 0 {
		return "", 0, errInvalidPostgresMigration
	}

	operation := args[0]
	switch operation {
	case migrationUp, migrationStatus, migrationVersion:
		if len(args) == 1 {
			return operation, 0, nil
		}
	case migrationDown:
		if len(args) != 2 {
			break
		}
		steps, err := strconv.Atoi(args[1])
		if err != nil || steps <= 0 {
			return "", 0, fmt.Errorf("invalid rollback steps %q: must be a positive integer: %w", args[1], errInvalidPostgresMigration)
		}
		return operation, steps, nil
	default:
		return "", 0, fmt.Errorf("unknown migration operation %q: %w", operation, errInvalidPostgresMigration)
	}
	return "", 0, errInvalidPostgresMigration
}

func runPostgresMigrations(args []string) int {
	operation, steps, err := parsePostgresMigrationCommand(args)
	if err != nil {
		return invalidMigrationArguments(err)
	}
	return runMigrationCommand("postgres", operation, func(ctx context.Context, cfg config.Config) (string, error) {
		return migrations.RunWithConfig(ctx, cfg.Database, operation, steps)
	})
}
