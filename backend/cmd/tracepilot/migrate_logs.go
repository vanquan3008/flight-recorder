package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

func runClickHouseMigrations(arguments []string) int {
	if len(arguments) != 1 || (arguments[0] != "up" && arguments[0] != "backfill") {
		migrationUsage()
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		return 2
	}
	store, err := clickhouse.New(cfg.ClickHouse, cfg.Retention.MaxAge)
	if err != nil {
		slog.Error("invalid ClickHouse configuration", "error", err)
		return 2
	}
	defer store.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if arguments[0] == "up" {
		err = store.Migrate(ctx)
	} else if err = store.Ready(ctx); err == nil {
		var database *postgres.Database
		database, err = postgres.Open(ctx, cfg.Database)
		if err == nil {
			defer database.Close()
			if err = database.Ready(ctx); err == nil {
				var count int
				count, err = backfillLogs(ctx, database, store)
				slog.Info("log backfill progress", "rows_copied", count)
			}
		}
	}
	if err != nil {
		slog.Error("ClickHouse migration failed", "error", err)
		return 1
	}
	slog.Info("ClickHouse migration completed", "operation", arguments[0])
	return 0
}

func backfillLogs(ctx context.Context, database *postgres.Database, store *clickhouse.Store) (int, error) {
	scopes, err := database.ListScopes(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, scope := range scopes {
		filter := query.LogFilter{EnvironmentID: scope.Environment.ID, Limit: 500}
		for {
			rows, err := database.ListLogs(ctx, scope.Project.ID, filter)
			if err != nil {
				return count, err
			}
			if err := store.StoreLogRecords(ctx, scope.Project.ID, scope.Environment.ID, rows); err != nil {
				return count, err
			}
			count += len(rows)
			if len(rows) <= filter.Limit {
				break
			}
			last := rows[len(rows)-1]
			filter.Before, filter.CursorID = last.OccurredAt, last.EventID
		}
	}
	return count, nil
}
