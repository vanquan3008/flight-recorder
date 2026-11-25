package schema

import (
	"context"
	"fmt"
	"slices"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/logs"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/traces"
)

func (store *Schema) Ready(ctx context.Context) error {
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
func (store *Schema) readySchema(ctx context.Context) error {
	for _, check := range []string{
		"SELECT " + logs.Columns + " FROM tracepilot_log_records LIMIT 0 FORMAT JSONEachRow",
		"SELECT " + traces.SpanColumns + " FROM tracepilot_spans LIMIT 0 FORMAT JSONEachRow",
		"SELECT project_id, environment_id, trace_id, started_at, ended_at FROM tracepilot_trace_index LIMIT 0 FORMAT JSONEachRow",
		"SELECT " + metrics.Columns + " FROM tracepilot_metric_points LIMIT 0 FORMAT JSONEachRow",
	} {
		if _, err := store.client.Execute(ctx, check, nil, nil, false); err != nil {
			return err
		}
	}
	type schemaObject struct{ Name, Engine string }
	objects, err := client.QueryRows[schemaObject](ctx, store.client, `SELECT name, engine FROM system.tables
WHERE database = {database:String} AND name IN ('tracepilot_trace_index_mv', 'tracepilot_span_minute_marks_mv',
'tracepilot_span_minutes', 'tracepilot_span_minute_marks', 'tracepilot_span_minute_done') FORMAT JSONEachRow`,
		map[string]string{"database": store.client.Config().Database})
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
