package main

import (
	"context"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

// Backfill is the explicit cross-database workflow, separate from either migration engine.
func migrateLogBackfill(ctx context.Context, cfg config.Config) (string, error) {
	store, err := clickhouse.New(cfg.ClickHouse, cfg.Retention.MaxAge)
	if err != nil {
		return "", err
	}
	defer store.Close()
	if err := store.Ready(ctx); err != nil {
		return "", err
	}
	database, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		return "", err
	}
	defer database.Close()
	if err := database.Ready(ctx); err != nil {
		return "", err
	}
	count, err := backfillLogs(ctx, database, store)
	return fmt.Sprintf("copied %d log row(s)", count), err
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
