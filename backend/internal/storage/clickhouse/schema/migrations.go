package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vanquan3008/tracepilot/db"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
)

func (store *Schema) MigrationStatus(ctx context.Context) (migrations.Status, error) {
	list, err := db.ListMigrations(db.ClickHouseMigrations)
	if err != nil {
		return migrations.Status{}, err
	}
	return store.migrationStatus(ctx, list)
}

func (store *Schema) migrationStatus(ctx context.Context, history []db.Migration) (migrations.Status, error) {
	state := migrations.NewStatus(history, 0, false)
	data, err := store.client.Execute(ctx, `SELECT name FROM system.tables
WHERE database = {database:String} AND name = 'tracepilot_schema_migrations' FORMAT JSONEachRow`,
		map[string]string{"database": store.client.Config().Database}, nil, false)
	if err != nil {
		return state, err
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		data, err = store.client.Execute(ctx, `SELECT version, CAST(countIf(dirty = false) = 0 AS Bool) AS dirty
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

// Migrate manages ClickHouse independently of PostgreSQL.
// shortcut: ClickHouse has no migration lock; use one updater for non-idempotent DDL.
func (store *Schema) Migrate(ctx context.Context) error {
	list, err := db.ListMigrations(db.ClickHouseMigrations)
	if err != nil {
		return err
	}
	if _, err := store.client.Execute(ctx, "CREATE DATABASE IF NOT EXISTS `"+store.client.Config().Database+"`", nil, nil, false); err != nil {
		return err
	}
	state, err := store.migrationStatus(ctx, list)
	if err != nil {
		return err
	}
	if state.Dirty || state.Version > state.Target {
		return fmt.Errorf("ClickHouse migration requires repair or a newer binary: %s", state.String())
	}
	if _, err := store.client.Execute(ctx, `CREATE TABLE IF NOT EXISTS tracepilot_schema_migrations (
version UInt64, dirty Bool) ENGINE = MergeTree ORDER BY (version, dirty)`, nil, nil, false); err != nil {
		return err
	}
	for _, migration := range list {
		if migration.Version <= state.Version {
			continue
		}
		data, err := db.MigrationFiles.ReadFile(migration.FilePath)
		if err != nil {
			return err
		}
		if err := store.recordMigration(ctx, migration.Version, true); err != nil {
			return err
		}
		if err := store.applyMigration(ctx, string(data)); err != nil {
			return fmt.Errorf("ClickHouse migration %d: %w", migration.Version, err)
		}
		if err := store.recordMigration(ctx, migration.Version, false); err != nil {
			return err
		}
	}
	if len(state.Pending) == 0 {
		return store.readySchema(ctx)
	}
	return nil
}

func (store *Schema) recordMigration(ctx context.Context, version uint, dirty bool) error {
	_, err := store.client.Execute(ctx, fmt.Sprintf("INSERT INTO tracepilot_schema_migrations VALUES (%d, %t)", version, dirty), nil, nil, false)
	return err
}

func (store *Schema) applyMigration(ctx context.Context, ddl string) error {
	ddl = strings.ReplaceAll(ddl, "__PARTITION__", store.client.Config().LogPartition.PartitionExpr())
	_, err := store.client.Execute(ctx, ddl, nil, nil, false)
	return err
}
