package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/db"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
)

func (store *Store) MigrationStatus(ctx context.Context) (migrations.Status, error) {
	list, err := db.List(db.ClickHouseMigrations)
	if err != nil {
		return migrations.Status{}, err
	}
	return store.migrationStatus(ctx, list)
}

func (store *Store) migrationStatus(ctx context.Context, history []db.Migration) (migrations.Status, error) {
	state := migrations.NewStatus(history, 0, false)
	data, err := store.execute(ctx, `SELECT name FROM system.tables
WHERE database = {database:String} AND name = 'tracepilot_schema_migrations' FORMAT JSONEachRow`,
		map[string]string{"database": store.cfg.Database}, nil, false)
	if err != nil {
		return state, err
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		data, err = store.execute(ctx, `SELECT version, CAST(countIf(dirty = false) = 0 AS Bool) AS dirty
FROM tracepilot_schema_migrations GROUP BY version ORDER BY version DESC LIMIT 1 FORMAT JSONEachRow`, nil, nil, false)
		if err != nil {
			return state, err
		}
		if len(strings.TrimSpace(string(data))) != 0 {
			var row struct {
				Version uint `json:"version"`
				Dirty   bool `json:"dirty"`
			}
			if err := json.Unmarshal(data, &row); err != nil {
				return state, fmt.Errorf("read ClickHouse migration status: %w", err)
			}
			state.Version, state.Dirty = row.Version, row.Dirty
		}
	}
	return migrations.NewStatus(history, state.Version, state.Dirty), nil
}

// RunMigration opens a short-lived client and applies ("up") or reports ("status") the log schema.
func RunMigration(ctx context.Context, cfg config.ClickHouseConfig, maxAge time.Duration, operation string) (string, error) {
	if operation != "up" && operation != "status" {
		return "", fmt.Errorf("unsupported ClickHouse migration operation %q", operation)
	}
	store, err := New(cfg, maxAge)
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

// Migrate manages ClickHouse independently of PostgreSQL.
// shortcut: ClickHouse has no migration lock; use one updater for non-idempotent DDL.
func (store *Store) Migrate(ctx context.Context) error {
	list, err := db.List(db.ClickHouseMigrations)
	if err != nil {
		return err
	}
	if _, err := store.execute(ctx, "CREATE DATABASE IF NOT EXISTS `"+store.cfg.Database+"`", nil, nil, false); err != nil {
		return err
	}
	state, err := store.migrationStatus(ctx, list)
	if err != nil {
		return err
	}
	if state.Dirty || state.Version > state.Target {
		return fmt.Errorf("ClickHouse migration requires repair or a newer binary: %s", state.String())
	}
	if _, err := store.execute(ctx, `CREATE TABLE IF NOT EXISTS tracepilot_schema_migrations (
version UInt64, dirty Bool) ENGINE = MergeTree ORDER BY (version, dirty)`, nil, nil, false); err != nil {
		return err
	}
	for _, migration := range list {
		if migration.Version <= state.Version {
			continue
		}
		data, err := db.SQL.ReadFile(migration.File)
		if err != nil {
			return err
		}
		if err := store.recordMigration(ctx, migration.Version, true); err != nil {
			return err
		}
		ddl := strings.NewReplacer("__PARTITION__", store.cfg.LogPartition.PartitionExpr(), "__TTL__", store.ttl()).Replace(string(data))
		if _, err := store.execute(ctx, ddl, nil, nil, false); err != nil {
			return fmt.Errorf("ClickHouse migration %d: %w", migration.Version, err)
		}
		if migration.Version == state.Target {
			// Validate legacy tables before adopting them into the version history.
			if err := store.prepareSchema(ctx); err != nil {
				return err
			}
		}
		if err := store.recordMigration(ctx, migration.Version, false); err != nil {
			return err
		}
	}
	if len(state.Pending) == 0 {
		return store.prepareSchema(ctx)
	}
	return nil
}

func (store *Store) recordMigration(ctx context.Context, version uint, dirty bool) error {
	_, err := store.execute(ctx, fmt.Sprintf("INSERT INTO tracepilot_schema_migrations VALUES (%d, %t)", version, dirty), nil, nil, false)
	return err
}

func (store *Store) prepareSchema(ctx context.Context) error {
	state, err := store.schema(ctx)
	if err != nil {
		return err
	}
	if change := store.ttlChange(state); change != "" {
		if _, err := store.execute(ctx, "ALTER TABLE tracepilot_logs "+change, nil, nil, false); err != nil {
			return err
		}
		return store.readySchema(ctx)
	}
	return store.checkLogColumns(ctx)
}
