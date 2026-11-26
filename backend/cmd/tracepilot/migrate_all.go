package main

import (
	"context"
	"errors"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func runAllMigrations(extraArguments []string) int {
	if len(extraArguments) != 0 {
		return invalidMigrationArguments(errors.New("migrate all takes no extra arguments"))
	}
	return runMigrationCommand("all", "all", func(ctx context.Context, cfg config.Config) (string, error) {
		if err := migrateAll(ctx, cfg); err != nil {
			return "", err
		}
		return "both database schemas are ready", nil
	})
}
