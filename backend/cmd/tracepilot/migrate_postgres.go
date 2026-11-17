package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

const (
	migrationUp      = "up"
	migrationDown    = "down"
	migrationStatus  = "status"
	migrationVersion = "version"
)

var errInvalidPostgresMigration = errors.New(
	"expected: up, status, version, or down <positive-steps>",
)

func parsePostgresMigrationCommand(args []string) (string, int, error) {
	if len(args) == 0 {
		return "", 0, errInvalidPostgresMigration
	}

	switch args[0] {
	case migrationUp, migrationStatus, migrationVersion:
		if len(args) != 1 {
			return "", 0, errInvalidPostgresMigration
		}
		return args[0], 0, nil

	case migrationDown:
		if len(args) != 2 {
			return "", 0, errInvalidPostgresMigration
		}

		steps, err := strconv.Atoi(args[1])
		if err != nil || steps <= 0 {
			return "", 0, fmt.Errorf(
				"invalid rollback steps %q: must be a positive integer: %w",
				args[1],
				errInvalidPostgresMigration,
			)
		}

		return migrationDown, steps, nil

	default:
		return "", 0, fmt.Errorf(
			"unknown migration operation %q: %w",
			args[0],
			errInvalidPostgresMigration,
		)
	}
}

func runPostgresMigrations(args []string) int {
	operation, steps, err := parsePostgresMigrationCommand(args)
	if err != nil {
		return invalidMigrationArguments(err)
	}
	return runMigrationCommand(
		"postgres",
		operation,
		func(ctx context.Context, cfg config.Config) (string, error) {
			return migrations.RunWith(
				ctx,
				cfg.Database.URL,
				operation,
				steps,
				postgresMigrationOptions(cfg.Database),
			)
		},
	)
}

func postgresMigrationOptions(db config.DatabaseConfig) migrations.Options {
	return migrations.Options{
		ConnectTimeout:   db.ConnectTimeout,
		LockTimeout:      db.MigrationLockTimeout,
		StatementTimeout: db.MigrationStatementTimeout,
	}
}
