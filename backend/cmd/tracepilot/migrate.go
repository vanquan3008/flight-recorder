package main

import (
	"flag"
	"log/slog"
	"os"
	"strconv"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
)

func runMigrations(arguments []string) int {
	if len(arguments) > 0 && arguments[0] == "clickhouse" {
		return runClickHouseMigrations(arguments[1:])
	}
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if parseError := flags.Parse(arguments); parseError != nil {
		return 2
	}

	operationArguments := flags.Args()
	if len(operationArguments) == 0 {
		migrationUsage()
		return 2
	}

	operation := operationArguments[0]
	steps := 0
	switch operation {
	case "up", "version":
		if len(operationArguments) != 1 {
			migrationUsage()
			return 2
		}
	case "down":
		if len(operationArguments) != 2 {
			migrationUsage()
			return 2
		}
		parsedSteps, parseError := strconv.Atoi(operationArguments[1])
		if parseError != nil || parsedSteps <= 0 {
			slog.Error("migration down steps must be a positive integer")
			return 2
		}
		steps = parsedSteps
	default:
		migrationUsage()
		return 2
	}

	configuration, loadError := config.Load()
	if loadError != nil {
		slog.Error("invalid configuration", "error", loadError)
		return 2
	}

	result, migrationError := migrations.Run(configuration.Database.URL, configuration.Database.MigrationsPath, operation, steps)
	if migrationError != nil {
		slog.Error("migration failed", "error", migrationError)
		return 1
	}
	slog.Info("migration completed", "result", result)
	return 0
}

func migrationUsage() {
	slog.Error("invalid migration arguments", "usage", "tracepilot migrate <up|down STEPS|version|clickhouse up|clickhouse backfill>")
}
