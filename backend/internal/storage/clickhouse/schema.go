package clickhouse

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func (store *Store) Ready(ctx context.Context) error {
	state, err := store.MigrationStatus(ctx)
	if err != nil {
		return err
	}
	if state.Dirty || state.Version != state.Target {
		return fmt.Errorf("ClickHouse schema version is %d (dirty=%t), expected %d; inspect migrate clickhouse status or run migrate clickhouse up", state.Version, state.Dirty, state.Target)
	}
	return store.readySchema(ctx)
}

// readySchema checks the read columns and the materialized views that feed derived tables.
func (store *Store) readySchema(ctx context.Context) error {
	for _, check := range []string{
		"SELECT " + logColumns + " FROM tracepilot_log_records LIMIT 0 FORMAT JSONEachRow",
		"SELECT " + spanColumns + " FROM tracepilot_spans LIMIT 0 FORMAT JSONEachRow",
		"SELECT project_id, environment_id, trace_id, started_at, ended_at FROM tracepilot_trace_index LIMIT 0 FORMAT JSONEachRow",
		"SELECT " + metricColumns + " FROM tracepilot_metric_points LIMIT 0 FORMAT JSONEachRow",
	} {
		if _, err := store.execute(ctx, check, nil, nil, false); err != nil {
			return err
		}
	}
	type schemaObject struct{ Name, Engine string }
	objects, err := queryRows[schemaObject](ctx, store, `SELECT name, engine FROM system.tables
WHERE database = {database:String} AND name IN ('tracepilot_trace_index_mv', 'tracepilot_span_minute_marks_mv',
'tracepilot_span_minutes', 'tracepilot_span_minute_marks', 'tracepilot_span_minute_done') FORMAT JSONEachRow`,
		map[string]string{"database": store.cfg.Database})
	if err != nil {
		return err
	}
	for _, required := range []schemaObject{
		{"tracepilot_trace_index_mv", "MaterializedView"},
		{"tracepilot_span_minute_marks_mv", "MaterializedView"},
		{Name: "tracepilot_span_minutes"},
		{Name: "tracepilot_span_minute_marks"},
		{Name: "tracepilot_span_minute_done"},
	} {
		if !slices.ContainsFunc(objects, func(object schemaObject) bool {
			return object.Name == required.Name && (required.Engine == "" || object.Engine == required.Engine)
		}) {
			return fmt.Errorf("ClickHouse schema requires %s; check its definition and runtime grants", required.Name)
		}
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}

func validateScope(projectID, environmentID string) error {
	if !validUUID(projectID) || !validUUID(environmentID) {
		return fmt.Errorf("invalid ClickHouse scope: %w", telemetry.ErrInvalidEvent)
	}
	return nil
}
