package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vanquan3008/tracepilot/internal/config"
)

const (
	migrationUp      = "up"
	migrationDown    = "down"
	migrationStatus  = "status"
	migrationVersion = "version"
)

func runMigrations(arguments []string) int {
	if len(arguments) == 0 {
		return invalidMigrationArguments(errors.New("a migration operation is required"))
	}
	switch arguments[0] {
	case "postgres":
		if len(arguments) == 2 && arguments[1] == "all" {
			// Preserve the legacy spelling of the two-database command.
			return runAllMigrations(nil)
		}
		return runPostgresMigrations(arguments[1:])
	case "clickhouse":
		return runClickHouseMigrations(arguments[1:])
	case "all":
		return runAllMigrations(arguments[1:])
	default:
		return runPostgresMigrations(arguments)
	}
}

// Only CLI configuration, signals and reporting are shared; execution belongs to each database.
func runMigrationCommand(database, operation string, execute func(context.Context, config.Config) (string, error)) int {
	cfg, err := config.LoadMigrations(database)
	if err != nil {
		slog.Error("invalid configuration", "database", database, "error", err)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := execute(ctx, cfg)
	if err != nil {
		slog.Error("database migration failed", "database", database,
			"operation", operation, "result", result, "error", err)
		return 1
	}
	slog.Info("database migration completed", "database", database,
		"operation", operation, "result", result)
	return 0
}

func invalidMigrationArguments(err error) int {
	slog.Error("invalid migration arguments", "error", err,
		"usage", "tracepilot migrate <all|postgres up|postgres down STEPS|postgres version|postgres status|clickhouse up|clickhouse status> (postgres is the default)")
	return 2
}
